package resources

import (
	"context"
	"encoding/json"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                   = &SubscriptionResource{}
	_ resource.ResourceWithConfigure      = &SubscriptionResource{}
	_ resource.ResourceWithImportState    = &SubscriptionResource{}
	_ resource.ResourceWithValidateConfig = &SubscriptionResource{}
)

func NewSubscriptionResource() resource.Resource { return &SubscriptionResource{} }

type SubscriptionResource struct{ base }

type SubscriptionResourceModel struct {
	ID              types.String `tfsdk:"id"`
	ProjectID       types.String `tfsdk:"project_id"`
	Name            types.String `tfsdk:"name"`
	EndpointID      types.String `tfsdk:"endpoint_id"`
	SourceID        types.String `tfsdk:"source_id"`
	Function        types.String `tfsdk:"function"`
	DeliveryMode    types.String `tfsdk:"delivery_mode"`
	Type            types.String `tfsdk:"type"`
	FilterConfig    types.Object `tfsdk:"filter_config"`
	AlertConfig     types.Object `tfsdk:"alert_config"`
	RetryConfig     types.Object `tfsdk:"retry_config"`
	RateLimitConfig types.Object `tfsdk:"rate_limit_config"`
	CreatedAt       types.String `tfsdk:"created_at"`
}

type filterConfigModel struct {
	EventTypes types.List   `tfsdk:"event_types"`
	Filter     types.Object `tfsdk:"filter"`
}

type filterDocsModel struct {
	Headers jsontypes.Normalized `tfsdk:"headers"`
	Body    jsontypes.Normalized `tfsdk:"body"`
	Path    jsontypes.Normalized `tfsdk:"path"`
	Query   jsontypes.Normalized `tfsdk:"query"`
}

type alertConfigModel struct {
	Count     types.Int64  `tfsdk:"count"`
	Threshold types.String `tfsdk:"threshold"`
}

type retryConfigModel struct {
	Type       types.String `tfsdk:"type"`
	Duration   types.Int64  `tfsdk:"duration"`
	RetryCount types.Int64  `tfsdk:"retry_count"`
}

func jsonDocAttr(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, CustomType: jsontypes.NormalizedType{},
		Description: desc + " A JSON object (use `jsonencode()`); supports Convoy filter operators such as `$gte`, `$in`, `$or`, `$regex`."}
}

var (
	filterDocsAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Payload filters. Only events matching every document are delivered.",
		Attributes: map[string]schema.Attribute{
			"headers": jsonDocAttr("Match on request headers."),
			"body":    jsonDocAttr("Match on the event payload."),
			"path":    jsonDocAttr("Match on URL path parameters."),
			"query":   jsonDocAttr("Match on URL query parameters."),
		},
	}
	filterConfigAttr = objectOC("Event type and payload filtering.", map[string]schema.Attribute{
		"event_types": schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType,
			Description:   "Event types delivered by the subscription. Defaults to `[\"*\"]` (all).",
			PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
		"filter": filterDocsAttr,
	})
	alertConfigAttr = objectOC("Failure alerting.", map[string]schema.Attribute{
		"count":     int64OC("Number of failed deliveries that triggers an alert."),
		"threshold": stringOC("Go duration over which failures are counted, e.g. `1h`."),
	})
	retryConfigAttr = objectOC("Retry policy, overriding the project strategy.", map[string]schema.Attribute{
		"type": schema.StringAttribute{Optional: true, Computed: true,
			Description:   "Retry strategy, `linear` or `exponential`.",
			Validators:    []validator.String{stringvalidator.OneOf("linear", "exponential")},
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"duration":    int64OC("Seconds between retries."),
		"retry_count": int64OC("Maximum number of retries."),
	})
	subscriptionRateLimitAttr = objectOC("Delivery rate limit.", map[string]schema.Attribute{
		"count":    int64OC("Deliveries allowed per `duration`."),
		"duration": int64OC("Rate limit window, in seconds."),
	})
)

func (r *SubscriptionResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_subscription"
}

func (r *SubscriptionResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A subscription routes events to an endpoint. In outgoing projects it connects an endpoint to the event " +
			"types it should receive; in incoming projects it connects a source to the endpoint that receives its events.",
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute("subscription"),
			"project_id":  projectIDAttribute(),
			"name":        schema.StringAttribute{Required: true, Description: "Subscription name."},
			"endpoint_id": schema.StringAttribute{Required: true, Description: "Endpoint that receives the events."},
			"source_id":   optStr("Source the events come from. Required in incoming projects."),
			"function": optStr("JavaScript `transform` function applied to the payload before delivery. " +
				"Requires a Convoy edition with transformations. Convoy does not support removing a function once set."),
			"delivery_mode": schema.StringAttribute{Optional: true, Computed: true,
				Description:   "Delivery guarantee, `at_least_once` (default) or `at_most_once`.",
				Validators:    []validator.String{stringvalidator.OneOf("at_least_once", "at_most_once")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"type":              schema.StringAttribute{Computed: true, Description: "Subscription type (`api` for subscriptions created through the API).", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"filter_config":     filterConfigAttr,
			"alert_config":      alertConfigAttr,
			"retry_config":      retryConfigAttr,
			"rate_limit_config": subscriptionRateLimitAttr,
			"created_at":        timestampAttribute("Creation timestamp."),
		},
	}
}

func (r *SubscriptionResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var fc types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("filter_config"), &fc)...)
	var m filterConfigModel
	if !objectAs(ctx, fc, &m, &resp.Diagnostics) {
		return
	}
	var docs filterDocsModel
	if objectAs(ctx, m.Filter, &docs, &resp.Diagnostics) {
		validateFilterDocs(path.Root("filter_config").AtName("filter"), docs, &resp.Diagnostics)
	}
}

func validateFilterDocs(p path.Path, d filterDocsModel, diags *diag.Diagnostics) {
	for name, v := range map[string]jsontypes.Normalized{"headers": d.Headers, "body": d.Body, "path": d.Path, "query": d.Query} {
		if !shared.Known(v) {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(v.ValueString()), &obj); err != nil || obj == nil {
			diags.AddAttributeError(p.AtName(name), "Invalid filter document", "The filter must be a JSON object, e.g. jsonencode({ type = \"invoice.paid\" }).")
		}
	}
}

// ------------------------------------------------------------- conversions

func (r *SubscriptionResource) request(ctx context.Context, m *SubscriptionResourceModel, diags *diag.Diagnostics) convoy.SubscriptionRequest {
	req := convoy.SubscriptionRequest{
		Name: m.Name.ValueString(), EndpointID: m.EndpointID.ValueString(), SourceID: m.SourceID.ValueString(),
		Function: shared.OptStr(m.Function), DeliveryMode: m.DeliveryMode.ValueString(),
	}
	var fc filterConfigModel
	if objectAs(ctx, m.FilterConfig, &fc, diags) {
		req.FilterConfig = &convoy.FilterConfig{EventTypes: shared.StringList(ctx, fc.EventTypes, diags)}
		var docs filterDocsModel
		if objectAs(ctx, fc.Filter, &docs, diags) {
			req.FilterConfig.Filter = &convoy.FilterSchema{
				Headers: shared.JSONRaw(docs.Headers), Body: shared.JSONRaw(docs.Body),
				Path: shared.JSONRaw(docs.Path), Query: shared.JSONRaw(docs.Query),
			}
		}
		// Send empty documents so filters removed from configuration are cleared.
		if req.FilterConfig.Filter == nil {
			req.FilterConfig.Filter = &convoy.FilterSchema{}
		}
		f := req.FilterConfig.Filter
		for _, doc := range []*json.RawMessage{&f.Headers, &f.Body, &f.Path, &f.Query} {
			if *doc == nil {
				*doc = json.RawMessage("{}")
			}
		}
	}
	var ac alertConfigModel
	if objectAs(ctx, m.AlertConfig, &ac, diags) {
		req.AlertConfig = &convoy.AlertConfig{Count: ac.Count.ValueInt64(), Threshold: ac.Threshold.ValueString()}
	}
	var rc retryConfigModel
	if objectAs(ctx, m.RetryConfig, &rc, diags) {
		req.RetryConfig = &convoy.RetryConfigReq{Type: rc.Type.ValueString(), IntervalSeconds: rc.Duration.ValueInt64(), RetryCount: rc.RetryCount.ValueInt64()}
	}
	var rl rateLimitModel
	if objectAs(ctx, m.RateLimitConfig, &rl, diags) {
		req.RateLimitConfig = &convoy.RateLimitConfig{Count: rl.Count.ValueInt64(), Duration: rl.Duration.ValueInt64()}
	}
	return req
}

func filterConfigFromAPI(ctx context.Context, fc *convoy.FilterConfig, prior types.Object, diags *diag.Diagnostics) types.Object {
	if fc == nil {
		fc = &convoy.FilterConfig{}
	}
	var p filterConfigModel
	objectAs(ctx, prior, &p, diags)
	var pd filterDocsModel
	objectAs(ctx, p.Filter, &pd, diags)

	docs := types.ObjectNull(objectAttrs(filterDocsAttr))
	if f := fc.Filter; f != nil && !(shared.IsEmptyJSON(f.Headers) && shared.IsEmptyJSON(f.Body) && shared.IsEmptyJSON(f.Path) && shared.IsEmptyJSON(f.Query)) {
		docs = objectFrom(ctx, objectAttrs(filterDocsAttr), filterDocsModel{
			Headers: shared.FilterDocFromAPI(pd.Headers, f.Headers), Body: shared.FilterDocFromAPI(pd.Body, f.Body),
			Path: shared.FilterDocFromAPI(pd.Path, f.Path), Query: shared.FilterDocFromAPI(pd.Query, f.Query),
		}, diags)
	} else if shared.Known(p.Filter) {
		// Configured but all documents empty: keep the configured empty documents.
		docs = p.Filter
	}
	eventTypes := fc.EventTypes
	if eventTypes == nil {
		eventTypes = []string{}
	}
	return objectFrom(ctx, objectAttrs(filterConfigAttr), filterConfigModel{EventTypes: shared.ListOfStrings(eventTypes), Filter: docs}, diags)
}

func (r *SubscriptionResource) apply(ctx context.Context, s *convoy.Subscription, m *SubscriptionResourceModel, refresh bool, diags *diag.Diagnostics) {
	m.ID = types.StringValue(s.UID)
	if s.ProjectID != "" {
		m.ProjectID = types.StringValue(s.ProjectID)
	}
	merge(&m.Name, types.StringValue(s.Name), refresh)
	merge(&m.EndpointID, types.StringValue(s.EndpointID), refresh)
	merge(&m.SourceID, shared.StrOrNull(s.SourceID), refresh)
	merge(&m.Function, shared.StrPtrOrNull(emptyToNil(s.Function)), refresh)
	merge(&m.DeliveryMode, types.StringValue(s.DeliveryMode), refresh)
	m.Type = types.StringValue(s.Type)
	merge(&m.FilterConfig, filterConfigFromAPI(ctx, s.FilterConfig, m.FilterConfig, diags), refresh)

	ac := convoy.AlertConfig{}
	if s.AlertConfig != nil {
		ac = *s.AlertConfig
	}
	merge(&m.AlertConfig, objectFrom(ctx, objectAttrs(alertConfigAttr), alertConfigModel{
		Count: types.Int64Value(ac.Count), Threshold: types.StringValue(ac.Threshold)}, diags), refresh)

	rc := convoy.RetryConfig{}
	if s.RetryConfig != nil {
		rc = *s.RetryConfig
	}
	merge(&m.RetryConfig, objectFrom(ctx, objectAttrs(retryConfigAttr), retryConfigModel{
		Type: types.StringValue(rc.Type), Duration: types.Int64Value(rc.Duration), RetryCount: types.Int64Value(rc.RetryCount)}, diags), refresh)

	rl := convoy.RateLimitConfig{}
	if s.RateLimitConfig != nil {
		rl = *s.RateLimitConfig
	}
	merge(&m.RateLimitConfig, objectFrom(ctx, objectAttrs(subscriptionRateLimitAttr), rateLimitModel{
		Count: types.Int64Value(rl.Count), Duration: types.Int64Value(rl.Duration)}, diags), refresh)

	m.CreatedAt = types.StringValue(s.CreatedAt)
}

// fillUnknownNested resolves unknown values nested inside planned objects
// (e.g. alert_config.threshold when only count is configured) from the API.
func (r *SubscriptionResource) fillUnknownNested(ctx context.Context, s *convoy.Subscription, m *SubscriptionResourceModel, diags *diag.Diagnostics) {
	fresh := *m
	fresh.FilterConfig, fresh.AlertConfig, fresh.RetryConfig, fresh.RateLimitConfig =
		types.ObjectUnknown(objectAttrs(filterConfigAttr)), types.ObjectUnknown(objectAttrs(alertConfigAttr)),
		types.ObjectUnknown(objectAttrs(retryConfigAttr)), types.ObjectUnknown(objectAttrs(subscriptionRateLimitAttr))
	r.apply(ctx, s, &fresh, false, diags)
	m.FilterConfig = mergeObject(m.FilterConfig, fresh.FilterConfig)
	m.AlertConfig = mergeObject(m.AlertConfig, fresh.AlertConfig)
	m.RetryConfig = mergeObject(m.RetryConfig, fresh.RetryConfig)
	m.RateLimitConfig = mergeObject(m.RateLimitConfig, fresh.RateLimitConfig)
}

// ------------------------------------------------------------------- CRUD

func (r *SubscriptionResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	body := r.request(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	createBody := body
	createBody.RetryConfig = nil
	s, err := r.client().CreateSubscription(ctx, projectID, createBody)
	if err != nil {
		apiError(&resp.Diagnostics, "create subscription", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), s.UID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)

	// The create API does not accept retry_config; apply it with an update.
	if body.RetryConfig != nil {
		if _, err := r.client().UpdateSubscription(ctx, projectID, s.UID, body); err != nil {
			apiError(&resp.Diagnostics, "set subscription retry_config", err)
			return
		}
	}
	if s, err = r.client().GetSubscription(ctx, projectID, s.UID); err != nil {
		apiError(&resp.Diagnostics, "read subscription after create", err)
		return
	}
	r.fillUnknownNested(ctx, s, &plan, &resp.Diagnostics)
	r.apply(ctx, s, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client().GetSubscription(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "subscription") {
			return
		}
		apiError(&resp.Diagnostics, "read subscription", err)
		return
	}
	r.apply(ctx, s, &state, true, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SubscriptionResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SubscriptionResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, id := state.ProjectID.ValueString(), state.ID.ValueString()
	body := r.request(ctx, &plan, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client().UpdateSubscription(ctx, projectID, id, body); err != nil {
		apiError(&resp.Diagnostics, "update subscription", err)
		return
	}
	s, err := r.client().GetSubscription(ctx, projectID, id)
	if err != nil {
		apiError(&resp.Diagnostics, "read subscription after update", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.fillUnknownNested(ctx, s, &plan, &resp.Diagnostics)
	r.apply(ctx, s, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SubscriptionResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SubscriptionResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client().DeleteSubscription(ctx, state.ProjectID.ValueString(), state.ID.ValueString()); err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete subscription", err)
	}
}

func (r *SubscriptionResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, r.data, req, resp)
}
