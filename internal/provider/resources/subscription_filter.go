package resources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                   = &SubscriptionFilterResource{}
	_ resource.ResourceWithConfigure      = &SubscriptionFilterResource{}
	_ resource.ResourceWithImportState    = &SubscriptionFilterResource{}
	_ resource.ResourceWithValidateConfig = &SubscriptionFilterResource{}
)

func NewSubscriptionFilterResource() resource.Resource { return &SubscriptionFilterResource{} }

type SubscriptionFilterResource struct{ base }

type SubscriptionFilterResourceModel struct {
	ID             types.String         `tfsdk:"id"`
	ProjectID      types.String         `tfsdk:"project_id"`
	SubscriptionID types.String         `tfsdk:"subscription_id"`
	EventType      types.String         `tfsdk:"event_type"`
	Headers        jsontypes.Normalized `tfsdk:"headers"`
	Body           jsontypes.Normalized `tfsdk:"body"`
	Path           jsontypes.Normalized `tfsdk:"path"`
	Query          jsontypes.Normalized `tfsdk:"query"`
	EnabledAt      types.String         `tfsdk:"enabled_at"`
}

func (r *SubscriptionFilterResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription_filter"
}

func (r *SubscriptionFilterResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A per-event-type payload filter attached to a subscription. Use it instead of " +
			"`convoy_subscription.filter_config.filter` when different event types need different filters. " +
			"Do not manage the same event type's filter from both places.",
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute("filter"),
			"project_id": projectIDAttribute(),
			"subscription_id": schema.StringAttribute{Required: true, Description: "Subscription the filter belongs to. Changing this forces a new filter.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"event_type": schema.StringAttribute{Required: true, Description: "Event type the filter applies to, e.g. `invoice.paid`, or `*`."},
			"headers":    jsonDocAttr("Match on request headers."),
			"body":       jsonDocAttr("Match on the event payload."),
			"path":       jsonDocAttr("Match on URL path parameters."),
			"query":      jsonDocAttr("Match on URL query parameters."),
			"enabled_at": schema.StringAttribute{Computed: true, Description: "Time the filter became active.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *SubscriptionFilterResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m SubscriptionFilterResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	validateFilterDocs(path.Empty(), filterDocsModel{Headers: m.Headers, Body: m.Body, Path: m.Path, Query: m.Query}, &resp.Diagnostics)
}

func emptyIfNil(raw json.RawMessage) json.RawMessage {
	if raw == nil {
		return json.RawMessage("{}")
	}
	return raw
}

func (r *SubscriptionFilterResource) request(m *SubscriptionFilterResourceModel) convoy.FilterRequest {
	return convoy.FilterRequest{
		EventType: m.EventType.ValueString(),
		Headers:   emptyIfNil(shared.JSONRaw(m.Headers)), Body: emptyIfNil(shared.JSONRaw(m.Body)),
		Path: emptyIfNil(shared.JSONRaw(m.Path)), Query: emptyIfNil(shared.JSONRaw(m.Query)),
	}
}

func firstNonEmpty(a, b json.RawMessage) json.RawMessage {
	if !shared.IsEmptyJSON(a) {
		return a
	}
	return b
}

func (r *SubscriptionFilterResource) apply(f *convoy.Filter, m *SubscriptionFilterResourceModel, refresh bool) {
	m.ID = types.StringValue(f.UID)
	if f.SubscriptionID != "" {
		m.SubscriptionID = types.StringValue(f.SubscriptionID)
	}
	merge(&m.EventType, types.StringValue(f.EventType), refresh)
	merge(&m.Headers, shared.FilterDocFromAPI(m.Headers, firstNonEmpty(f.RawHeaders, f.Headers)), refresh)
	merge(&m.Body, shared.FilterDocFromAPI(m.Body, firstNonEmpty(f.RawBody, f.Body)), refresh)
	merge(&m.Path, shared.FilterDocFromAPI(m.Path, firstNonEmpty(f.RawPath, f.Path)), refresh)
	merge(&m.Query, shared.FilterDocFromAPI(m.Query, firstNonEmpty(f.RawQuery, f.Query)), refresh)
	m.EnabledAt = shared.StrOrNull(f.EnabledAt)
}

func (r *SubscriptionFilterResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SubscriptionFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := r.client().CreateFilter(ctx, projectID, plan.SubscriptionID.ValueString(), r.request(&plan))
	if err != nil {
		apiError(&resp.Diagnostics, "create subscription filter", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	r.apply(f, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionFilterResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SubscriptionFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := r.client().GetFilter(ctx, state.ProjectID.ValueString(), state.SubscriptionID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "subscription filter") {
			return
		}
		apiError(&resp.Diagnostics, "read subscription filter", err)
		return
	}
	r.apply(f, &state, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SubscriptionFilterResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SubscriptionFilterResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	f, err := r.client().UpdateFilter(ctx, state.ProjectID.ValueString(), state.SubscriptionID.ValueString(), state.ID.ValueString(), r.request(&plan))
	if err != nil {
		apiError(&resp.Diagnostics, "update subscription filter", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.apply(f, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionFilterResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SubscriptionFilterResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	err := r.client().DeleteFilter(ctx, state.ProjectID.ValueString(), state.SubscriptionID.ValueString(), state.ID.ValueString())
	if err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete subscription filter", err)
	}
}

func (r *SubscriptionFilterResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	def := ""
	if r.data != nil {
		def = r.data.ProjectID
	}
	parts, err := shared.ParseImportID(req.ID, 3, def)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("subscription_id"), parts[1])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
}
