package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                = &EventTypeResource{}
	_ resource.ResourceWithConfigure   = &EventTypeResource{}
	_ resource.ResourceWithImportState = &EventTypeResource{}
)

func NewEventTypeResource() resource.Resource { return &EventTypeResource{} }

type EventTypeResource struct{ base }

type EventTypeResourceModel struct {
	ID           types.String         `tfsdk:"id"`
	ProjectID    types.String         `tfsdk:"project_id"`
	Name         types.String         `tfsdk:"name"`
	Category     types.String         `tfsdk:"category"`
	Description  types.String         `tfsdk:"description"`
	JSONSchema   jsontypes.Normalized `tfsdk:"json_schema"`
	DeprecatedAt types.String         `tfsdk:"deprecated_at"`
}

func (r *EventTypeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_event_type"
}

func (r *EventTypeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An event type documents an event your project emits (e.g. `invoice.paid`) so customers can subscribe to it " +
			"from the portal. Convoy cannot delete event types: destroying this resource **deprecates** the event type, and " +
			"its name cannot be reused afterwards.",
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute("event type"),
			"project_id": projectIDAttribute(),
			"name": schema.StringAttribute{Required: true, Description: "Event type name, e.g. `invoice.paid`. Changing this forces a new event type (the old one is deprecated).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"category":    optStr("Product specific grouping, e.g. `billing`."),
			"description": optStr("What the event means."),
			"json_schema": schema.StringAttribute{Optional: true, CustomType: jsontypes.NormalizedType{},
				Description: "JSON Schema of the event payload (use `jsonencode()` or `file()`)."},
			"deprecated_at": schema.StringAttribute{Computed: true, Description: "Deprecation timestamp, when deprecated outside Terraform.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		},
	}
}

func (r *EventTypeResource) apply(e *convoy.EventType, m *EventTypeResourceModel, refresh bool) {
	m.ID = types.StringValue(e.UID)
	merge(&m.Name, types.StringValue(e.Name), refresh)
	merge(&m.Category, shared.StrOrNull(e.Category), refresh)
	merge(&m.Description, shared.StrOrNull(e.Description), refresh)
	merge(&m.JSONSchema, shared.JSONFromAPI(m.JSONSchema, e.JSONSchema), refresh)
	m.DeprecatedAt = shared.StrOrNull(e.DeprecatedAt)
}

func (r *EventTypeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EventTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client().CreateEventType(ctx, projectID, convoy.EventTypeRequest{
		Name: plan.Name.ValueString(), Category: plan.Category.ValueString(), Description: plan.Description.ValueString(),
		JSONSchema: shared.JSONRaw(plan.JSONSchema),
	})
	if err != nil {
		apiError(&resp.Diagnostics, "create event type", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	r.apply(e, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EventTypeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EventTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client().GetEventType(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "event type") {
			return
		}
		apiError(&resp.Diagnostics, "read event type", err)
		return
	}
	r.apply(e, &state, true)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *EventTypeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EventTypeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client().UpdateEventType(ctx, state.ProjectID.ValueString(), state.ID.ValueString(), convoy.EventTypeRequest{
		Category: plan.Category.ValueString(), Description: plan.Description.ValueString(), JSONSchema: shared.JSONRaw(plan.JSONSchema),
	})
	if err != nil {
		apiError(&resp.Diagnostics, "update event type", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.apply(e, &plan, false)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EventTypeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EventTypeResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if shared.Known(state.DeprecatedAt) {
		return
	}
	_, err := r.client().DeprecateEventType(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "deprecate event type", err)
		return
	}
	resp.Diagnostics.AddWarning("Event type deprecated",
		"Convoy has no API to delete event types; \""+state.Name.ValueString()+"\" was deprecated instead and its name cannot be reused.")
}

func (r *EventTypeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, r.data, req, resp)
}
