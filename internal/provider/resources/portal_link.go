package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                   = &PortalLinkResource{}
	_ resource.ResourceWithConfigure      = &PortalLinkResource{}
	_ resource.ResourceWithImportState    = &PortalLinkResource{}
	_ resource.ResourceWithValidateConfig = &PortalLinkResource{}
)

func NewPortalLinkResource() resource.Resource { return &PortalLinkResource{} }

type PortalLinkResource struct{ base }

type PortalLinkResourceModel struct {
	ID                types.String `tfsdk:"id"`
	ProjectID         types.String `tfsdk:"project_id"`
	Name              types.String `tfsdk:"name"`
	OwnerID           types.String `tfsdk:"owner_id"`
	Endpoints         types.Set    `tfsdk:"endpoints"`
	CanManageEndpoint types.Bool   `tfsdk:"can_manage_endpoint"`
	AuthType          types.String `tfsdk:"auth_type"`
	URL               types.String `tfsdk:"url"`
	Token             types.String `tfsdk:"token"`
	EndpointCount     types.Int64  `tfsdk:"endpoint_count"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

func (r *PortalLinkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portal_link"
}

func (r *PortalLinkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A portal link gives one of your customers a hosted UI to inspect deliveries and (optionally) manage the " +
			"endpoints that share an owner ID. Destroying the resource revokes the link.",
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute("portal link"),
			"project_id": projectIDAttribute(),
			"name":       schema.StringAttribute{Required: true, Description: "Portal link name."},
			"owner_id":   optStr("Owner ID; the portal shows every endpoint with this owner ID. Either `owner_id` or `endpoints` is required."),
			"endpoints": schema.SetAttribute{Optional: true, ElementType: types.StringType,
				Description: "Explicit endpoint IDs to expose. Deprecated by Convoy in favour of `owner_id`."},
			"can_manage_endpoint": boolOC("Allow the customer to create and edit endpoints from the portal."),
			"auth_type": schema.StringAttribute{Optional: true, Computed: true,
				Description: "Portal authentication: `static_token` (long-lived token in the URL) or `refresh_token` (short-lived tokens, " +
					"see the `convoy_portal_link_token` ephemeral resource). Changing this forces a new portal link.",
				Validators:    []validator.String{stringvalidator.OneOf("static_token", "refresh_token")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"url": schema.StringAttribute{Computed: true, Sensitive: true, Description: "Portal URL to share with the customer (contains the access token).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"token": schema.StringAttribute{Computed: true, Sensitive: true, Description: "Portal access token.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"endpoint_count": schema.Int64Attribute{Computed: true, Description: "Number of endpoints visible in the portal."},
			"created_at":     timestampAttribute("Creation timestamp."),
		},
	}
}

func (r *PortalLinkResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m PortalLinkResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if m.OwnerID.IsNull() && m.Endpoints.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("owner_id"), "Missing portal link target", "Set owner_id (recommended) or endpoints.")
	}
}

func (r *PortalLinkResource) request(ctx context.Context, m *PortalLinkResourceModel, resp *[]string) convoy.PortalLinkRequest {
	return convoy.PortalLinkRequest{
		Name: m.Name.ValueString(), OwnerID: m.OwnerID.ValueString(), Endpoints: *resp,
		CanManageEndpoint: m.CanManageEndpoint.ValueBool(), AuthType: m.AuthType.ValueString(),
	}
}

func (r *PortalLinkResource) apply(p *convoy.PortalLink, m *PortalLinkResourceModel, refresh bool) {
	m.ID = types.StringValue(p.UID)
	if p.ProjectID != "" {
		m.ProjectID = types.StringValue(p.ProjectID)
	}
	merge(&m.Name, types.StringValue(p.Name), refresh)
	merge(&m.OwnerID, shared.StrOrNull(p.OwnerID), refresh)
	if refresh && !m.Endpoints.IsNull() {
		// Only track the endpoint list when it is configured: with owner_id
		// Convoy reports every endpoint of the owner here.
		m.Endpoints = shared.SetOfStrings(p.Endpoints)
	}
	merge(&m.CanManageEndpoint, types.BoolValue(p.CanManageEndpoint), refresh)
	merge(&m.AuthType, types.StringValue(p.AuthType), refresh)
	if p.URL != "" {
		m.URL = types.StringValue(p.URL)
	} else if m.URL.IsUnknown() {
		m.URL = types.StringNull()
	}
	if p.Token != "" {
		m.Token = types.StringValue(p.Token)
	} else if m.Token.IsUnknown() {
		m.Token = types.StringNull()
	}
	m.EndpointCount = types.Int64Value(p.EndpointCount)
	m.CreatedAt = types.StringValue(p.CreatedAt)
}

func (r *PortalLinkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan PortalLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	endpoints := shared.StringList(ctx, plan.Endpoints, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client().CreatePortalLink(ctx, projectID, r.request(ctx, &plan, &endpoints))
	if err != nil {
		apiError(&resp.Diagnostics, "create portal link", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	r.apply(p, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PortalLinkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state PortalLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client().GetPortalLink(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "portal link") {
			return
		}
		apiError(&resp.Diagnostics, "read portal link", err)
		return
	}
	r.apply(p, &state, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *PortalLinkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state PortalLinkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	endpoints := shared.StringList(ctx, plan.Endpoints, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, id := state.ProjectID.ValueString(), state.ID.ValueString()
	if _, err := r.client().UpdatePortalLink(ctx, projectID, id, r.request(ctx, &plan, &endpoints)); err != nil {
		apiError(&resp.Diagnostics, "update portal link", err)
		return
	}
	p, err := r.client().GetPortalLink(ctx, projectID, id)
	if err != nil {
		apiError(&resp.Diagnostics, "read portal link after update", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.apply(p, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *PortalLinkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state PortalLinkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client().RevokePortalLink(ctx, state.ProjectID.ValueString(), state.ID.ValueString()); err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "revoke portal link", err)
	}
}

func (r *PortalLinkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, r.data, req, resp)
}
