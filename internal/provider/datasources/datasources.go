// Package datasources contains the read-only data sources of the Convoy provider.
package datasources

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-validators/datasourcevalidator"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

type base struct{ data *shared.ProviderData }

func (b *base) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	b.data = shared.FromProviderData(req.ProviderData, &resp.Diagnostics)
}

func projectIDAttr() schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Computed: true, Description: "Project ID. Defaults to the provider's `project_id`."}
}

// ================================================================ project

var _ datasource.DataSourceWithConfigure = &ProjectDataSource{}

func NewProjectDataSource() datasource.DataSource { return &ProjectDataSource{} }

type ProjectDataSource struct{ base }

type projectModel struct {
	ID             types.String `tfsdk:"id"`
	Name           types.String `tfsdk:"name"`
	Type           types.String `tfsdk:"type"`
	OrganisationID types.String `tfsdk:"organisation_id"`
	LogoURL        types.String `tfsdk:"logo_url"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

func (d *ProjectDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (d *ProjectDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a Convoy project. Works with a project API key, which makes it a handy way to inspect the project the key belongs to.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Optional: true, Computed: true, Description: "Project ID. Defaults to the provider's `project_id`."},
			"name":            schema.StringAttribute{Computed: true, Description: "Project name."},
			"type":            schema.StringAttribute{Computed: true, Description: "`outgoing` or `incoming`."},
			"organisation_id": schema.StringAttribute{Computed: true, Description: "Owning organisation."},
			"logo_url":        schema.StringAttribute{Computed: true, Description: "Logo URL."},
			"created_at":      schema.StringAttribute{Computed: true, Description: "Creation timestamp."},
		},
	}
}

func (d *ProjectDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m projectModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	id := d.data.ResolveProjectID(m.ID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := d.data.Client.GetProject(ctx, id)
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to read project %s: %s", id, err))
		return
	}
	m.ID, m.Name, m.Type = types.StringValue(p.UID), types.StringValue(p.Name), types.StringValue(p.Type)
	m.OrganisationID, m.LogoURL, m.CreatedAt = types.StringValue(p.OrganisationID), types.StringValue(p.LogoURL), types.StringValue(p.CreatedAt)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// =============================================================== endpoint

var (
	_ datasource.DataSourceWithConfigure        = &EndpointDataSource{}
	_ datasource.DataSourceWithConfigValidators = &EndpointDataSource{}
)

func NewEndpointDataSource() datasource.DataSource { return &EndpointDataSource{} }

type EndpointDataSource struct{ base }

type endpointModel struct {
	ID          types.String `tfsdk:"id"`
	ProjectID   types.String `tfsdk:"project_id"`
	Name        types.String `tfsdk:"name"`
	URL         types.String `tfsdk:"url"`
	Description types.String `tfsdk:"description"`
	OwnerID     types.String `tfsdk:"owner_id"`
	Status      types.String `tfsdk:"status"`
	Secret      types.String `tfsdk:"secret"`
}

func (d *EndpointDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (d *EndpointDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up an endpoint by `id` or by exact `name`.",
		Attributes: map[string]schema.Attribute{
			"id":          schema.StringAttribute{Optional: true, Computed: true, Description: "Endpoint ID."},
			"project_id":  projectIDAttr(),
			"name":        schema.StringAttribute{Optional: true, Computed: true, Description: "Exact endpoint name. Must match exactly one endpoint."},
			"url":         schema.StringAttribute{Computed: true, Description: "Endpoint URL."},
			"description": schema.StringAttribute{Computed: true, Description: "Description."},
			"owner_id":    schema.StringAttribute{Computed: true, Description: "Owner ID."},
			"status":      schema.StringAttribute{Computed: true, Description: "`active`, `inactive` or `paused`."},
			"secret":      schema.StringAttribute{Computed: true, Sensitive: true, Description: "Active signing secret."},
		},
	}
}

func (d *EndpointDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *EndpointDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m endpointModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := d.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var e *convoy.Endpoint
	var err error
	if shared.Known(m.ID) {
		e, err = d.data.Client.GetEndpoint(ctx, projectID, m.ID.ValueString())
	} else {
		var all []convoy.Endpoint
		all, err = d.data.Client.ListEndpoints(ctx, projectID, m.Name.ValueString(), "")
		if err == nil {
			e, err = exactlyOne(all, func(x convoy.Endpoint) bool { return x.Name == m.Name.ValueString() }, "endpoint", m.Name.ValueString())
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to read endpoint: %s", err))
		return
	}
	m.ID, m.ProjectID, m.Name, m.URL = types.StringValue(e.UID), types.StringValue(projectID), types.StringValue(e.Name), types.StringValue(e.URL)
	m.Description, m.OwnerID, m.Status = types.StringValue(e.Description), types.StringValue(e.OwnerID), types.StringValue(e.Status)
	m.Secret = types.StringValue(e.ActiveSecret())
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

func exactlyOne[T any](all []T, match func(T) bool, what, name string) (*T, error) {
	var found []T
	for _, x := range all {
		if match(x) {
			found = append(found, x)
		}
	}
	switch len(found) {
	case 0:
		return nil, fmt.Errorf("no %s named %q", what, name)
	case 1:
		return &found[0], nil
	default:
		return nil, fmt.Errorf("%d %ss are named %q; look it up by id instead", len(found), what, name)
	}
}

// ================================================================= source

var (
	_ datasource.DataSourceWithConfigure        = &SourceDataSource{}
	_ datasource.DataSourceWithConfigValidators = &SourceDataSource{}
)

func NewSourceDataSource() datasource.DataSource { return &SourceDataSource{} }

type SourceDataSource struct{ base }

type sourceModel struct {
	ID        types.String `tfsdk:"id"`
	ProjectID types.String `tfsdk:"project_id"`
	Name      types.String `tfsdk:"name"`
	Type      types.String `tfsdk:"type"`
	Provider  types.String `tfsdk:"source_provider"`
	URL       types.String `tfsdk:"url"`
	MaskID    types.String `tfsdk:"mask_id"`
}

func (d *SourceDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_source"
}

func (d *SourceDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Look up a source by `id` or by exact `name`, e.g. to retrieve its ingest URL.",
		Attributes: map[string]schema.Attribute{
			"id":              schema.StringAttribute{Optional: true, Computed: true, Description: "Source ID."},
			"project_id":      projectIDAttr(),
			"name":            schema.StringAttribute{Optional: true, Computed: true, Description: "Exact source name. Must match exactly one source."},
			"type":            schema.StringAttribute{Computed: true, Description: "Source type."},
			"source_provider": schema.StringAttribute{Computed: true, Description: "Pre-configured provider, if any."},
			"url":             schema.StringAttribute{Computed: true, Description: "Ingest URL."},
			"mask_id":         schema.StringAttribute{Computed: true, Description: "Public identifier used in the ingest URL."},
		},
	}
}

func (d *SourceDataSource) ConfigValidators(context.Context) []datasource.ConfigValidator {
	return []datasource.ConfigValidator{datasourcevalidator.ExactlyOneOf(path.MatchRoot("id"), path.MatchRoot("name"))}
}

func (d *SourceDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m sourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := d.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	var s *convoy.Source
	var err error
	if shared.Known(m.ID) {
		s, err = d.data.Client.GetSource(ctx, projectID, m.ID.ValueString())
	} else {
		var all []convoy.Source
		all, err = d.data.Client.ListSources(ctx, projectID)
		if err == nil {
			s, err = exactlyOne(all, func(x convoy.Source) bool { return x.Name == m.Name.ValueString() }, "source", m.Name.ValueString())
		}
	}
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to read source: %s", err))
		return
	}
	m.ID, m.ProjectID, m.Name, m.Type = types.StringValue(s.UID), types.StringValue(projectID), types.StringValue(s.Name), types.StringValue(s.Type)
	m.Provider, m.URL, m.MaskID = types.StringValue(s.Provider), types.StringValue(s.URL), types.StringValue(s.MaskID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}

// ============================================================ event types

var _ datasource.DataSourceWithConfigure = &EventTypesDataSource{}

func NewEventTypesDataSource() datasource.DataSource { return &EventTypesDataSource{} }

type EventTypesDataSource struct{ base }

type eventTypesModel struct {
	ProjectID         types.String     `tfsdk:"project_id"`
	IncludeDeprecated types.Bool       `tfsdk:"include_deprecated"`
	EventTypes        []eventTypeModel `tfsdk:"event_types"`
	Names             types.List       `tfsdk:"names"`
}

type eventTypeModel struct {
	ID           types.String `tfsdk:"id"`
	Name         types.String `tfsdk:"name"`
	Category     types.String `tfsdk:"category"`
	Description  types.String `tfsdk:"description"`
	JSONSchema   types.String `tfsdk:"json_schema"`
	DeprecatedAt types.String `tfsdk:"deprecated_at"`
}

func (d *EventTypesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_event_types"
}

func (d *EventTypesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "List the event types of a project.",
		Attributes: map[string]schema.Attribute{
			"project_id":         projectIDAttr(),
			"include_deprecated": schema.BoolAttribute{Optional: true, Description: "Include deprecated event types. Defaults to `false`."},
			"names":              schema.ListAttribute{Computed: true, ElementType: types.StringType, Description: "Event type names, convenient for `filter_config.event_types`."},
			"event_types": schema.ListNestedAttribute{
				Computed:    true,
				Description: "Event types.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"id":            schema.StringAttribute{Computed: true, Description: "Event type ID."},
					"name":          schema.StringAttribute{Computed: true, Description: "Name."},
					"category":      schema.StringAttribute{Computed: true, Description: "Category."},
					"description":   schema.StringAttribute{Computed: true, Description: "Description."},
					"json_schema":   schema.StringAttribute{Computed: true, Description: "JSON schema, as a JSON string."},
					"deprecated_at": schema.StringAttribute{Computed: true, Description: "Deprecation timestamp, if deprecated."},
				}},
			},
		},
	}
}

func (d *EventTypesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var m eventTypesModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := d.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	all, err := d.data.Client.ListEventTypes(ctx, projectID)
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to list event types: %s", err))
		return
	}
	m.ProjectID = types.StringValue(projectID)
	m.EventTypes = []eventTypeModel{}
	names := []string{}
	for _, e := range all {
		if e.DeprecatedAt != "" && !m.IncludeDeprecated.ValueBool() {
			continue
		}
		schemaStr := ""
		if !shared.IsEmptyJSON(e.JSONSchema) {
			schemaStr = string(e.JSONSchema)
		}
		m.EventTypes = append(m.EventTypes, eventTypeModel{
			ID: types.StringValue(e.UID), Name: types.StringValue(e.Name), Category: types.StringValue(e.Category),
			Description: types.StringValue(e.Description), JSONSchema: types.StringValue(schemaStr), DeprecatedAt: types.StringValue(e.DeprecatedAt),
		})
		names = append(names, e.Name)
	}
	m.Names = shared.ListOfStrings(names)
	resp.Diagnostics.Append(resp.State.Set(ctx, &m)...)
}
