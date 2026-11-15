package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/objectplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                = &ProjectResource{}
	_ resource.ResourceWithConfigure   = &ProjectResource{}
	_ resource.ResourceWithImportState = &ProjectResource{}
)

func NewProjectResource() resource.Resource { return &ProjectResource{} }

type ProjectResource struct{ base }

type ProjectResourceModel struct {
	ID             types.String `tfsdk:"id"`
	OrganisationID types.String `tfsdk:"organisation_id"`
	Name           types.String `tfsdk:"name"`
	Type           types.String `tfsdk:"type"`
	LogoURL        types.String `tfsdk:"logo_url"`
	Config         types.Object `tfsdk:"config"`
	APIKey         types.String `tfsdk:"api_key"`
	CreatedAt      types.String `tfsdk:"created_at"`
}

// ------------------------------------------------------------ nested models

type projectConfigModel struct {
	MaxPayloadReadSize             types.Int64  `tfsdk:"max_payload_read_size"`
	ReplayAttacksPreventionEnabled types.Bool   `tfsdk:"replay_attacks_prevention_enabled"`
	AddEventIDTraceHeaders         types.Bool   `tfsdk:"add_event_id_trace_headers"`
	DisableEndpoint                types.Bool   `tfsdk:"disable_endpoint"`
	MultipleEndpointSubscriptions  types.Bool   `tfsdk:"multiple_endpoint_subscriptions"`
	SearchPolicy                   types.String `tfsdk:"search_policy"`
	SSL                            types.Object `tfsdk:"ssl"`
	RateLimit                      types.Object `tfsdk:"ratelimit"`
	Strategy                       types.Object `tfsdk:"strategy"`
	Signature                      types.Object `tfsdk:"signature"`
	MetaEvent                      types.Object `tfsdk:"meta_event"`
	CircuitBreaker                 types.Object `tfsdk:"circuit_breaker"`
}

type sslModel struct {
	EnforceSecureEndpoints types.Bool `tfsdk:"enforce_secure_endpoints"`
}

type rateLimitModel struct {
	Count    types.Int64 `tfsdk:"count"`
	Duration types.Int64 `tfsdk:"duration"`
}

type strategyModel struct {
	Type       types.String `tfsdk:"type"`
	Duration   types.Int64  `tfsdk:"duration"`
	RetryCount types.Int64  `tfsdk:"retry_count"`
}

type signatureModel struct {
	Header   types.String `tfsdk:"header"`
	Versions types.List   `tfsdk:"versions"`
}

type signatureVersionModel struct {
	Hash     types.String `tfsdk:"hash"`
	Encoding types.String `tfsdk:"encoding"`
}

type metaEventModel struct {
	IsEnabled  types.Bool   `tfsdk:"is_enabled"`
	Type       types.String `tfsdk:"type"`
	EventTypes types.List   `tfsdk:"event_type"`
	URL        types.String `tfsdk:"url"`
	Secret     types.String `tfsdk:"secret"`
}

type circuitBreakerModel struct {
	SampleRate                  types.Int64 `tfsdk:"sample_rate"`
	ErrorTimeout                types.Int64 `tfsdk:"error_timeout"`
	FailureThreshold            types.Int64 `tfsdk:"failure_threshold"`
	SuccessThreshold            types.Int64 `tfsdk:"success_threshold"`
	ObservabilityWindow         types.Int64 `tfsdk:"observability_window"`
	MinimumRequestCount         types.Int64 `tfsdk:"minimum_request_count"`
	ConsecutiveFailureThreshold types.Int64 `tfsdk:"consecutive_failure_threshold"`
}

// ----------------------------------------------------------------- schema

func int64OC(desc string) schema.Int64Attribute {
	return schema.Int64Attribute{Optional: true, Computed: true, Description: desc,
		PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()}}
}

func boolOC(desc string) schema.BoolAttribute {
	return schema.BoolAttribute{Optional: true, Computed: true, Description: desc,
		PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}}
}

func objectOC(desc string, attrs map[string]schema.Attribute) schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{Optional: true, Computed: true, Description: desc, Attributes: attrs,
		PlanModifiers: []planmodifier.Object{objectplanmodifier.UseStateForUnknown()}}
}

var (
	sslAttr = objectOC("Endpoint SSL enforcement rules.", map[string]schema.Attribute{
		"enforce_secure_endpoints": boolOC("Only allow `https` endpoint URLs."),
	})
	rateLimitAttr = objectOC("Project wide rate limiting.", map[string]schema.Attribute{
		"count":    int64OC("Number of requests allowed per `duration`."),
		"duration": int64OC("Rate limit window, in seconds."),
	})
	strategyAttr = objectOC("Retry strategy for failing deliveries.", map[string]schema.Attribute{
		"type": schema.StringAttribute{Optional: true, Computed: true,
			Description:   "Retry strategy, either `linear` or `exponential`.",
			Validators:    []validator.String{stringvalidator.OneOf("linear", "exponential")},
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"duration":    int64OC("Seconds between retries (base interval for `exponential`)."),
		"retry_count": int64OC("Maximum number of retries."),
	})
	signatureAttr = objectOC("Signature header and versions used to sign outgoing webhooks.", map[string]schema.Attribute{
		"header": stringOC("Signature header name, e.g. `X-Convoy-Signature`."),
		"versions": schema.ListNestedAttribute{
			Optional: true, Computed: true,
			Description:   "Signature versions, oldest first. Convoy signs with every version listed. Existing versions keep their identity when they stay at the same position.",
			PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
				"hash": schema.StringAttribute{Required: true, Description: "Hash function, e.g. `SHA256` or `SHA512`."},
				"encoding": schema.StringAttribute{Required: true, Description: "Signature encoding, `hex` or `base64`.",
					Validators: []validator.String{stringvalidator.OneOf("hex", "base64")}},
			}},
		},
	})
	metaEventAttr = objectOC("Meta events: Convoy sends notifications about the project's own activity (endpoint changes, delivery outcomes...).", map[string]schema.Attribute{
		"is_enabled": boolOC("Whether meta events are enabled."),
		"type": schema.StringAttribute{Optional: true, Computed: true, Description: "Delivery type, `http`.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"event_type": schema.ListAttribute{Optional: true, Computed: true, ElementType: types.StringType,
			Description:   "Meta event types to send, e.g. `endpoint.created`, `eventdelivery.success`.",
			PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()}},
		"url": stringOC("URL meta events are delivered to."),
		"secret": schema.StringAttribute{Optional: true, Computed: true, Sensitive: true,
			Description:   "Secret used to sign meta events.",
			PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
	})
	circuitBreakerAttr = objectOC("Circuit breaker that disables endpoints that keep failing.", map[string]schema.Attribute{
		"sample_rate":                   int64OC("How often (seconds) the breaker samples delivery results."),
		"error_timeout":                 int64OC("Seconds a tripped breaker stays open."),
		"failure_threshold":             int64OC("Failure percentage that trips the breaker."),
		"success_threshold":             int64OC("Success percentage needed to close a half-open breaker."),
		"observability_window":          int64OC("Window, in minutes, over which failures are counted."),
		"minimum_request_count":         int64OC("Minimum requests in the window before the breaker can trip."),
		"consecutive_failure_threshold": int64OC("Consecutive trips after which the endpoint is disabled."),
	})
	projectConfigAttr = objectOC("Project configuration. Omitted settings keep Convoy's current (default) values.", map[string]schema.Attribute{
		"max_payload_read_size":             int64OC("Maximum number of bytes read from an ingested or sent payload."),
		"replay_attacks_prevention_enabled": boolOC("Add a timestamp to the signature header to prevent replay attacks."),
		"add_event_id_trace_headers":        boolOC("Add event ID and event delivery ID headers to dispatched requests."),
		"disable_endpoint":                  boolOC("Disable endpoints once the retry threshold of an event is reached."),
		"multiple_endpoint_subscriptions":   boolOC("Allow several subscriptions per endpoint."),
		"search_policy":                     stringOC("Go duration (e.g. `720h`) of event retention for search."),
		"ssl":                               sslAttr,
		"ratelimit":                         rateLimitAttr,
		"strategy":                          strategyAttr,
		"signature":                         signatureAttr,
		"meta_event":                        metaEventAttr,
		"circuit_breaker":                   circuitBreakerAttr,
	})
	signatureVersionAttrTypes = signatureAttr.Attributes["versions"].(schema.ListNestedAttribute).NestedObject.Type().(types.ObjectType).AttrTypes
)

func (r *ProjectResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_project"
}

func (r *ProjectResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A Convoy project is an isolated workspace holding endpoints, sources, subscriptions and events. " +
			"`outgoing` projects send webhooks to your customers; `incoming` projects receive webhooks from providers. " +
			"Managing projects requires a personal API key (project API keys cannot create projects).",
		Attributes: map[string]schema.Attribute{
			"id": idAttribute("project"),
			"organisation_id": schema.StringAttribute{
				Optional: true, Computed: true,
				Description:   "Organisation that owns the project. Defaults to the provider's `organisation_id`. Changing this forces a new project.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
			},
			"name": schema.StringAttribute{Required: true, Description: "Project name."},
			"type": schema.StringAttribute{
				Required:      true,
				Description:   "Project type, `outgoing` or `incoming`. Changing this forces a new project.",
				Validators:    []validator.String{stringvalidator.OneOf("outgoing", "incoming")},
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"logo_url": schema.StringAttribute{Optional: true, Description: "URL of the project logo."},
			"config":   projectConfigAttr,
			"api_key": schema.StringAttribute{
				Computed: true, Sensitive: true,
				Description:   "Project API key generated when the project is created. Only returned at creation; empty for imported projects.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"created_at": timestampAttribute("Creation timestamp."),
		},
	}
}

// ------------------------------------------------------------- conversions

// projectConfigToAPI builds the config to send. Convoy replaces nested
// objects (strategy, ratelimit, ...) wholesale, so every nested object is
// built on top of base (the server's current config) and only the planned,
// known fields are overlaid. When base is nil (project creation), nested
// objects with unknown fields are reported as partial and left out; the
// caller applies them in a follow-up update once the defaults are known.
func projectConfigToAPI(ctx context.Context, o types.Object, base *convoy.ProjectConfig, diags *diag.Diagnostics) (cfg *convoy.ProjectConfig, partial bool) {
	var m projectConfigModel
	if !objectAs(ctx, o, &m, diags) {
		return nil, false
	}
	if base == nil {
		base = &convoy.ProjectConfig{}
	}
	cfg = &convoy.ProjectConfig{
		MaxPayloadReadSize:             shared.OptInt64(m.MaxPayloadReadSize),
		ReplayAttacksPreventionEnabled: shared.OptBool(m.ReplayAttacksPreventionEnabled),
		AddEventIDTraceHeaders:         shared.OptBool(m.AddEventIDTraceHeaders),
		DisableEndpoint:                shared.OptBool(m.DisableEndpoint),
		MultipleEndpointSubscriptions:  shared.OptBool(m.MultipleEndpointSubscriptions),
		SearchPolicy:                   shared.OptStr(m.SearchPolicy),
	}
	baseKnown := base.Strategy != nil || base.SSL != nil || base.RateLimit != nil
	// overlay reports whether a nested object can be sent: it is known and
	// either has no unknown fields or the server's base values fill them.
	overlay := func(obj types.Object, target any, hasBase bool) bool {
		if !objectAs(ctx, obj, target, diags) {
			return false
		}
		if hasUnknown(obj) && !hasBase {
			partial = true
			return false
		}
		return true
	}

	var ssl sslModel
	if overlay(m.SSL, &ssl, base.SSL != nil || baseKnown) {
		v := convoy.SSLConfig{}
		if base.SSL != nil {
			v = *base.SSL
		}
		setBool(&v.EnforceSecureEndpoints, ssl.EnforceSecureEndpoints)
		cfg.SSL = &v
	}
	var rl rateLimitModel
	if overlay(m.RateLimit, &rl, base.RateLimit != nil) {
		v := convoy.RateLimitConfig{}
		if base.RateLimit != nil {
			v = *base.RateLimit
		}
		setInt(&v.Count, rl.Count)
		setInt(&v.Duration, rl.Duration)
		cfg.RateLimit = &v
	}
	var st strategyModel
	if overlay(m.Strategy, &st, base.Strategy != nil) {
		v := convoy.StrategyConfig{}
		if base.Strategy != nil {
			v = *base.Strategy
		}
		setStr(&v.Type, st.Type)
		setInt(&v.Duration, st.Duration)
		setInt(&v.RetryCount, st.RetryCount)
		cfg.Strategy = &v
	}
	var sig signatureModel
	if objectAs(ctx, m.Signature, &sig, diags) {
		v := convoy.SignatureConfig{}
		if base.Signature != nil {
			v = *base.Signature
		}
		setStr(&v.Header, sig.Header)
		if shared.Known(sig.Versions) {
			var versions []signatureVersionModel
			diags.Append(sig.Versions.ElementsAs(ctx, &versions, false)...)
			v.Versions = nil
			for i, pv := range versions {
				sv := convoy.SignatureVersion{Hash: pv.Hash.ValueString(), Encoding: pv.Encoding.ValueString()}
				// Keep identity of versions that already exist at this position.
				if base.Signature != nil && i < len(base.Signature.Versions) {
					if b := base.Signature.Versions[i]; b.Hash == sv.Hash && b.Encoding == sv.Encoding {
						sv.UID, sv.CreatedAt = b.UID, b.CreatedAt
					}
				}
				v.Versions = append(v.Versions, sv)
			}
		}
		if v.Header == "" && base.Signature == nil {
			partial = true
		} else {
			cfg.Signature = &v
		}
	}
	var me metaEventModel
	if objectAs(ctx, m.MetaEvent, &me, diags) {
		v := convoy.MetaEventConfig{EventType: []string{}}
		if base.MetaEvent != nil {
			v = *base.MetaEvent
		}
		setBool(&v.IsEnabled, me.IsEnabled)
		setStr(&v.Type, me.Type)
		setStr(&v.URL, me.URL)
		setStr(&v.Secret, me.Secret)
		if shared.Known(me.EventTypes) {
			v.EventType = shared.StringList(ctx, me.EventTypes, diags)
		}
		if v.EventType == nil {
			v.EventType = []string{}
		}
		cfg.MetaEvent = &v
	}
	var cb circuitBreakerModel
	if overlay(m.CircuitBreaker, &cb, base.CircuitBreaker != nil) {
		v := convoy.CircuitBreakerConfig{}
		if base.CircuitBreaker != nil {
			v = *base.CircuitBreaker
		}
		setInt(&v.SampleRate, cb.SampleRate)
		setInt(&v.ErrorTimeout, cb.ErrorTimeout)
		setInt(&v.FailureThreshold, cb.FailureThreshold)
		setInt(&v.SuccessThreshold, cb.SuccessThreshold)
		setInt(&v.ObservabilityWindow, cb.ObservabilityWindow)
		setInt(&v.MinimumRequestCount, cb.MinimumRequestCount)
		setInt(&v.ConsecutiveFailureThreshold, cb.ConsecutiveFailureThreshold)
		cfg.CircuitBreaker = &v
	}
	return cfg, partial
}

func hasUnknown(o types.Object) bool {
	for _, v := range o.Attributes() {
		if v.IsUnknown() {
			return true
		}
		if n, ok := v.(types.Object); ok && hasUnknown(n) {
			return true
		}
	}
	return false
}

func setStr(dst *string, v types.String) {
	if shared.Known(v) {
		*dst = v.ValueString()
	}
}

func setInt(dst *int64, v types.Int64) {
	if shared.Known(v) {
		*dst = v.ValueInt64()
	}
}

func setBool(dst *bool, v types.Bool) {
	if shared.Known(v) {
		*dst = v.ValueBool()
	}
}

func boolVal(b *bool) types.Bool {
	if b == nil {
		return types.BoolValue(false)
	}
	return types.BoolValue(*b)
}

func int64Val(i *int64) types.Int64 {
	if i == nil {
		return types.Int64Value(0)
	}
	return types.Int64Value(*i)
}

func strVal(s *string) types.String {
	if s == nil {
		return types.StringValue("")
	}
	return types.StringValue(*s)
}

func projectConfigFromAPI(ctx context.Context, cfg *convoy.ProjectConfig, prior types.Object, diags *diag.Diagnostics) types.Object {
	attrs := objectAttrs(projectConfigAttr)
	if cfg == nil {
		cfg = &convoy.ProjectConfig{}
	}
	m := projectConfigModel{
		MaxPayloadReadSize:             int64Val(cfg.MaxPayloadReadSize),
		ReplayAttacksPreventionEnabled: boolVal(cfg.ReplayAttacksPreventionEnabled),
		AddEventIDTraceHeaders:         boolVal(cfg.AddEventIDTraceHeaders),
		DisableEndpoint:                boolVal(cfg.DisableEndpoint),
		MultipleEndpointSubscriptions:  boolVal(cfg.MultipleEndpointSubscriptions),
		SearchPolicy:                   strVal(cfg.SearchPolicy),
	}

	ssl := convoy.SSLConfig{}
	if cfg.SSL != nil {
		ssl = *cfg.SSL
	}
	m.SSL = objectFrom(ctx, objectAttrs(sslAttr), sslModel{EnforceSecureEndpoints: types.BoolValue(ssl.EnforceSecureEndpoints)}, diags)

	rl := convoy.RateLimitConfig{}
	if cfg.RateLimit != nil {
		rl = *cfg.RateLimit
	}
	m.RateLimit = objectFrom(ctx, objectAttrs(rateLimitAttr), rateLimitModel{Count: types.Int64Value(rl.Count), Duration: types.Int64Value(rl.Duration)}, diags)

	st := convoy.StrategyConfig{}
	if cfg.Strategy != nil {
		st = *cfg.Strategy
	}
	m.Strategy = objectFrom(ctx, objectAttrs(strategyAttr), strategyModel{
		Type: types.StringValue(st.Type), Duration: types.Int64Value(st.Duration), RetryCount: types.Int64Value(st.RetryCount)}, diags)

	sig := convoy.SignatureConfig{}
	if cfg.Signature != nil {
		sig = *cfg.Signature
	}
	versions := make([]attr.Value, 0, len(sig.Versions))
	for _, v := range sig.Versions {
		versions = append(versions, objectFrom(ctx, signatureVersionAttrTypes, signatureVersionModel{
			Hash: types.StringValue(v.Hash), Encoding: types.StringValue(v.Encoding),
		}, diags))
	}
	versionList, d := types.ListValue(types.ObjectType{AttrTypes: signatureVersionAttrTypes}, versions)
	diags.Append(d...)
	m.Signature = objectFrom(ctx, objectAttrs(signatureAttr), signatureModel{Header: types.StringValue(sig.Header), Versions: versionList}, diags)

	// The meta event secret is write-mostly: keep the configured value.
	var priorSecret types.String = types.StringNull()
	var pc projectConfigModel
	if objectAs(ctx, prior, &pc, diags) {
		var pme metaEventModel
		if objectAs(ctx, pc.MetaEvent, &pme, diags) {
			priorSecret = pme.Secret
		}
	}
	me := convoy.MetaEventConfig{}
	if cfg.MetaEvent != nil {
		me = *cfg.MetaEvent
	}
	secret := shared.KeepSensitive(priorSecret, me.Secret)
	if secret.IsNull() {
		secret = types.StringValue("")
	}
	m.MetaEvent = objectFrom(ctx, objectAttrs(metaEventAttr), metaEventModel{
		IsEnabled: types.BoolValue(me.IsEnabled), Type: types.StringValue(me.Type), URL: types.StringValue(me.URL),
		Secret: secret, EventTypes: optList(me.EventType),
	}, diags)

	cb := convoy.CircuitBreakerConfig{}
	if cfg.CircuitBreaker != nil {
		cb = *cfg.CircuitBreaker
	}
	m.CircuitBreaker = objectFrom(ctx, objectAttrs(circuitBreakerAttr), circuitBreakerModel{
		SampleRate: types.Int64Value(cb.SampleRate), ErrorTimeout: types.Int64Value(cb.ErrorTimeout),
		FailureThreshold: types.Int64Value(cb.FailureThreshold), SuccessThreshold: types.Int64Value(cb.SuccessThreshold),
		ObservabilityWindow: types.Int64Value(cb.ObservabilityWindow), MinimumRequestCount: types.Int64Value(cb.MinimumRequestCount),
		ConsecutiveFailureThreshold: types.Int64Value(cb.ConsecutiveFailureThreshold),
	}, diags)

	return objectFrom(ctx, attrs, m, diags)
}

func (r *ProjectResource) apply(ctx context.Context, p *convoy.Project, m *ProjectResourceModel, diags *diag.Diagnostics) {
	m.ID = types.StringValue(p.UID)
	if p.OrganisationID != "" {
		m.OrganisationID = types.StringValue(p.OrganisationID)
	}
	m.Name = types.StringValue(p.Name)
	m.Type = types.StringValue(p.Type)
	m.LogoURL = shared.StrOrNull(p.LogoURL)
	m.Config = projectConfigFromAPI(ctx, p.Config, m.Config, diags)
	m.CreatedAt = types.StringValue(p.CreatedAt)
	if m.APIKey.IsUnknown() {
		m.APIKey = types.StringNull()
	}
}

// ------------------------------------------------------------------- CRUD

func (r *ProjectResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	orgID := plan.OrganisationID.ValueString()
	if orgID == "" {
		orgID = r.data.OrganisationID
	}
	if orgID == "" {
		resp.Diagnostics.AddAttributeError(path.Root("organisation_id"), "Missing organisation ID",
			"Set organisation_id on the resource or on the provider (or CONVOY_ORGANISATION_ID).")
		return
	}
	cfg, partial := projectConfigToAPI(ctx, plan.Config, nil, &resp.Diagnostics)
	body := convoy.CreateProjectRequest{
		Name: plan.Name.ValueString(), Type: plan.Type.ValueString(), LogoURL: plan.LogoURL.ValueString(), Config: cfg,
	}
	if resp.Diagnostics.HasError() {
		return
	}
	out, err := r.client().CreateProject(ctx, orgID, body)
	if err != nil {
		apiError(&resp.Diagnostics, "create project", err)
		return
	}
	if out.Project == nil {
		resp.Diagnostics.AddError("Convoy API error", "create project response did not include the project")
		return
	}
	plan.APIKey = types.StringNull()
	if out.APIKey != nil && out.APIKey.Key != "" {
		plan.APIKey = types.StringValue(out.APIKey.Key)
	}
	plan.OrganisationID = types.StringValue(orgID)
	project := out.Project
	if partial {
		// Partially configured nested objects: overlay them on the defaults
		// Convoy just assigned.
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), project.UID)...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("api_key"), plan.APIKey)...)
		if project, err = r.client().GetProject(ctx, project.UID); err == nil {
			cfg, _ = projectConfigToAPI(ctx, plan.Config, project.Config, &resp.Diagnostics)
			if _, err = r.client().UpdateProject(ctx, project.UID, convoy.UpdateProjectRequest{Name: project.Name, LogoURL: project.LogoURL, Config: cfg}); err == nil {
				project, err = r.client().GetProject(ctx, project.UID)
			}
		}
		if err != nil {
			apiError(&resp.Diagnostics, "apply project config", err)
			return
		}
	}
	r.apply(ctx, project, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	p, err := r.client().GetProject(ctx, state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "project") {
			return
		}
		apiError(&resp.Diagnostics, "read project", err)
		return
	}
	r.apply(ctx, p, &state, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *ProjectResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state ProjectResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	current, err := r.client().GetProject(ctx, state.ID.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "read project before update", err)
		return
	}
	cfg, _ := projectConfigToAPI(ctx, plan.Config, current.Config, &resp.Diagnostics)
	if cfg == nil {
		cfg = current.Config
	}
	body := convoy.UpdateProjectRequest{Name: plan.Name.ValueString(), LogoURL: plan.LogoURL.ValueString(), Config: cfg}
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client().UpdateProject(ctx, state.ID.ValueString(), body); err != nil {
		apiError(&resp.Diagnostics, "update project", err)
		return
	}
	p, err := r.client().GetProject(ctx, state.ID.ValueString())
	if err != nil {
		apiError(&resp.Diagnostics, "read project after update", err)
		return
	}
	plan.ID, plan.APIKey = state.ID, state.APIKey
	r.apply(ctx, p, &plan, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ProjectResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ProjectResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client().DeleteProject(ctx, state.ID.ValueString()); err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete project", err)
	}
}

func (r *ProjectResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("config"), types.ObjectNull(objectAttrs(projectConfigAttr)))...)
}
