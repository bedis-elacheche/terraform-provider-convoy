// Package actions contains Terraform actions (Terraform 1.14+): imperative,
// one-off operations that do not map to a managed object, such as sending an
// event or rotating a secret. Like the Stripe provider's refund/payout
// actions, they can be run with `terraform apply -invoke=action.convoy_x.name`
// or attached to a resource lifecycle via `action_trigger`.
package actions

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/action"
	"github.com/hashicorp/terraform-plugin-framework/action/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

type base struct{ data *shared.ProviderData }

func (b *base) Configure(_ context.Context, req action.ConfigureRequest, resp *action.ConfigureResponse) {
	b.data = shared.FromProviderData(req.ProviderData, &resp.Diagnostics)
}

func progress(resp *action.InvokeResponse, format string, args ...any) {
	if resp.SendProgress != nil {
		resp.SendProgress(action.InvokeProgressEvent{Message: fmt.Sprintf(format, args...)})
	}
}

func projectIDAttr() schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: "Project ID. Defaults to the provider's `project_id`."}
}

// ============================================================= send event

var (
	_ action.ActionWithConfigure      = &SendEventAction{}
	_ action.ActionWithValidateConfig = &SendEventAction{}
)

func NewSendEventAction() action.Action { return &SendEventAction{} }

type SendEventAction struct{ base }

type sendEventModel struct {
	ProjectID      types.String         `tfsdk:"project_id"`
	Mode           types.String         `tfsdk:"mode"`
	EndpointID     types.String         `tfsdk:"endpoint_id"`
	OwnerID        types.String         `tfsdk:"owner_id"`
	EventType      types.String         `tfsdk:"event_type"`
	Data           jsontypes.Normalized `tfsdk:"data"`
	CustomHeaders  types.Map            `tfsdk:"custom_headers"`
	IdempotencyKey types.String         `tfsdk:"idempotency_key"`
}

func (a *SendEventAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_send_event"
}

func (a *SendEventAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Sends an event through Convoy: to one endpoint (`direct`), to every endpoint sharing an owner ID (`fanout`) " +
			"or to every subscribed endpoint (`broadcast`). Useful to smoke-test a freshly provisioned endpoint.",
		Attributes: map[string]schema.Attribute{
			"project_id": projectIDAttr(),
			"mode": schema.StringAttribute{Optional: true,
				Description: "`direct` (default, requires `endpoint_id`), `fanout` (requires `owner_id`) or `broadcast`.",
				Validators:  []validator.String{stringvalidator.OneOf("direct", "fanout", "broadcast")}},
			"endpoint_id": schema.StringAttribute{Optional: true, Description: "Target endpoint, for `direct` mode."},
			"owner_id":    schema.StringAttribute{Optional: true, Description: "Target owner ID, for `fanout` mode."},
			"event_type":  schema.StringAttribute{Required: true, Description: "Event type, e.g. `invoice.paid`."},
			"data": schema.StringAttribute{Required: true, CustomType: jsontypes.NormalizedType{},
				Description: "JSON payload (use `jsonencode()`)."},
			"custom_headers":  schema.MapAttribute{Optional: true, ElementType: types.StringType, Description: "Extra headers sent with the event."},
			"idempotency_key": schema.StringAttribute{Optional: true, Description: "Key used by Convoy to de-duplicate the event."},
		},
	}
}

func (a *SendEventAction) ValidateConfig(ctx context.Context, req action.ValidateConfigRequest, resp *action.ValidateConfigResponse) {
	var m sendEventModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() || m.Mode.IsUnknown() {
		return
	}
	switch mode := m.Mode.ValueString(); mode {
	case "", "direct":
		if m.EndpointID.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("endpoint_id"), "Missing endpoint_id", "endpoint_id is required in direct mode.")
		}
	case "fanout":
		if m.OwnerID.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("owner_id"), "Missing owner_id", "owner_id is required in fanout mode.")
		}
	}
}

func (a *SendEventAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var m sendEventModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := a.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	headers := shared.MapOfStrings(ctx, m.CustomHeaders, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	mode := m.Mode.ValueString()
	if mode == "" {
		mode = "direct"
	}
	ev, err := a.data.Client.SendEvent(ctx, projectID, mode, convoy.SendEventRequest{
		EndpointID: m.EndpointID.ValueString(), OwnerID: m.OwnerID.ValueString(), EventType: m.EventType.ValueString(),
		Data: json.RawMessage(m.Data.ValueString()), CustomHeaders: headers, IdempotencyKey: m.IdempotencyKey.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to send event: %s", err))
		return
	}
	if ev.UID != "" {
		progress(resp, "Sent %s event %s (%s)", mode, ev.UID, m.EventType.ValueString())
	} else {
		progress(resp, "Sent %s event (%s)", mode, m.EventType.ValueString())
	}
}

// ===================================================== secret rotation

var _ action.ActionWithConfigure = &EndpointSecretRotationAction{}

func NewEndpointSecretRotationAction() action.Action { return &EndpointSecretRotationAction{} }

type EndpointSecretRotationAction struct{ base }

type rotateModel struct {
	ProjectID  types.String `tfsdk:"project_id"`
	EndpointID types.String `tfsdk:"endpoint_id"`
	Secret     types.String `tfsdk:"secret"`
	Expiration types.Int64  `tfsdk:"expiration"`
}

func (a *EndpointSecretRotationAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint_secret_rotation"
}

func (a *EndpointSecretRotationAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Rotates an endpoint's signing secret: the current secret is expired after `expiration` and a new one " +
			"(supplied or generated by Convoy) becomes active. Run `terraform apply -refresh-only` afterwards to pick the new " +
			"secret up in a `convoy_endpoint` that does not set `secret` explicitly.",
		Attributes: map[string]schema.Attribute{
			"project_id":  projectIDAttr(),
			"endpoint_id": schema.StringAttribute{Required: true, Description: "Endpoint whose secret is rotated."},
			"secret":      schema.StringAttribute{Optional: true, Description: "New secret. Generated by Convoy when omitted. Action arguments are not persisted to state; pass an ephemeral or sensitive value."},
			"expiration":  schema.Int64Attribute{Optional: true, Description: "Grace period during which the previous secret remains valid. Defaults to `0`."},
		},
	}
}

func (a *EndpointSecretRotationAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var m rotateModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := a.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	_, err := a.data.Client.ExpireEndpointSecret(ctx, projectID, m.EndpointID.ValueString(), convoy.ExpireSecretRequest{
		Secret: m.Secret.ValueString(), Expiration: m.Expiration.ValueInt64(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to rotate endpoint secret: %s", err))
		return
	}
	progress(resp, "Rotated secret of endpoint %s", m.EndpointID.ValueString())
}

// ============================================================ event replay

var _ action.ActionWithConfigure = &EventReplayAction{}

func NewEventReplayAction() action.Action { return &EventReplayAction{} }

type EventReplayAction struct{ base }

type replayModel struct {
	ProjectID types.String `tfsdk:"project_id"`
	EventID   types.String `tfsdk:"event_id"`
}

func (a *EventReplayAction) Metadata(_ context.Context, req action.MetadataRequest, resp *action.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_event_replay"
}

func (a *EventReplayAction) Schema(_ context.Context, _ action.SchemaRequest, resp *action.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Replays an event: Convoy re-dispatches it to every endpoint it was originally sent to.",
		Attributes: map[string]schema.Attribute{
			"project_id": projectIDAttr(),
			"event_id":   schema.StringAttribute{Required: true, Description: "Event to replay."},
		},
	}
}

func (a *EventReplayAction) Invoke(ctx context.Context, req action.InvokeRequest, resp *action.InvokeResponse) {
	var m replayModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	projectID := a.data.ResolveProjectID(m.ProjectID, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := a.data.Client.ReplayEvent(ctx, projectID, m.EventID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Convoy API error", fmt.Sprintf("Unable to replay event: %s", err))
		return
	}
	progress(resp, "Replayed event %s", m.EventID.ValueString())
}
