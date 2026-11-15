package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-jsontypes/jsontypes"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
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
	_ resource.Resource                   = &EndpointResource{}
	_ resource.ResourceWithConfigure      = &EndpointResource{}
	_ resource.ResourceWithImportState    = &EndpointResource{}
	_ resource.ResourceWithValidateConfig = &EndpointResource{}
)

func NewEndpointResource() resource.Resource { return &EndpointResource{} }

type EndpointResource struct{ base }

type EndpointResourceModel struct {
	ID                 types.String `tfsdk:"id"`
	ProjectID          types.String `tfsdk:"project_id"`
	Name               types.String `tfsdk:"name"`
	URL                types.String `tfsdk:"url"`
	Description        types.String `tfsdk:"description"`
	OwnerID            types.String `tfsdk:"owner_id"`
	SupportEmail       types.String `tfsdk:"support_email"`
	SlackWebhookURL    types.String `tfsdk:"slack_webhook_url"`
	TeamsWebhookURL    types.String `tfsdk:"teams_webhook_url"`
	HTTPTimeout        types.Int64  `tfsdk:"http_timeout"`
	RateLimit          types.Int64  `tfsdk:"rate_limit"`
	RateLimitDuration  types.Int64  `tfsdk:"rate_limit_duration"`
	AdvancedSignatures types.Bool   `tfsdk:"advanced_signatures"`
	ContentType        types.String `tfsdk:"content_type"`
	Secret             types.String `tfsdk:"secret"`
	SecretExpiration   types.Int64  `tfsdk:"secret_expiration"`
	Paused             types.Bool   `tfsdk:"paused"`
	Status             types.String `tfsdk:"status"`
	Authentication     types.Object `tfsdk:"authentication"`
	MtlsClientCert     types.Object `tfsdk:"mtls_client_cert"`
	CreatedAt          types.String `tfsdk:"created_at"`
}

type endpointAuthModel struct {
	Type      types.String `tfsdk:"type"`
	APIKey    types.Object `tfsdk:"api_key"`
	BasicAuth types.Object `tfsdk:"basic_auth"`
	OAuth2    types.Object `tfsdk:"oauth2"`
}

type headerKeyModel struct {
	HeaderName  types.String `tfsdk:"header_name"`
	HeaderValue types.String `tfsdk:"header_value"`
}

type basicAuthModel struct {
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
}

type oauth2Model struct {
	URL                types.String         `tfsdk:"url"`
	ClientID           types.String         `tfsdk:"client_id"`
	AuthenticationType types.String         `tfsdk:"authentication_type"`
	ClientSecret       types.String         `tfsdk:"client_secret"`
	GrantType          types.String         `tfsdk:"grant_type"`
	Scope              types.String         `tfsdk:"scope"`
	Audience           types.String         `tfsdk:"audience"`
	Issuer             types.String         `tfsdk:"issuer"`
	Subject            types.String         `tfsdk:"subject"`
	SigningAlgorithm   types.String         `tfsdk:"signing_algorithm"`
	ExpiryTimeUnit     types.String         `tfsdk:"expiry_time_unit"`
	SigningKey         jsontypes.Normalized `tfsdk:"signing_key"`
	FieldMapping       types.Object         `tfsdk:"field_mapping"`
}

type fieldMappingModel struct {
	AccessToken types.String `tfsdk:"access_token"`
	TokenType   types.String `tfsdk:"token_type"`
	ExpiresIn   types.String `tfsdk:"expires_in"`
}

type mtlsModel struct {
	ClientCert types.String `tfsdk:"client_cert"`
	ClientKey  types.String `tfsdk:"client_key"`
}

func optStr(desc string) schema.StringAttribute {
	return schema.StringAttribute{Optional: true, Description: desc}
}

func sensitiveStr(desc string, required bool) schema.StringAttribute {
	return schema.StringAttribute{Optional: !required, Required: required, Sensitive: true, Description: desc}
}

var (
	headerKeyAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Static API key sent in a request header.",
		Attributes: map[string]schema.Attribute{
			"header_name":  schema.StringAttribute{Required: true, Description: "Header name, e.g. `Authorization`."},
			"header_value": sensitiveStr("Header value.", true),
		},
	}
	basicAuthAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "HTTP basic authentication credentials.",
		Attributes: map[string]schema.Attribute{
			"username": schema.StringAttribute{Required: true, Description: "Username."},
			"password": sensitiveStr("Password.", true),
		},
	}
	fieldMappingAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Custom field names used to parse the token response.",
		Attributes: map[string]schema.Attribute{
			"access_token": optStr("Field holding the access token, e.g. `accessToken`."),
			"token_type":   optStr("Field holding the token type."),
			"expires_in":   optStr("Field holding the expiry."),
		},
	}
	oauth2Attr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "OAuth2 client-credentials token exchange performed by Convoy before each delivery.",
		Attributes: map[string]schema.Attribute{
			"url":       schema.StringAttribute{Required: true, Description: "Token endpoint URL."},
			"client_id": schema.StringAttribute{Required: true, Description: "OAuth2 client ID."},
			"authentication_type": schema.StringAttribute{Required: true,
				Description: "How the client authenticates: `shared_secret` (requires `client_secret`) or `client_assertion` (requires `signing_key`).",
				Validators:  []validator.String{stringvalidator.OneOf("shared_secret", "client_assertion")}},
			"client_secret":     sensitiveStr("Client secret, for `shared_secret`.", false),
			"grant_type":        optStr("Grant type, defaults to `client_credentials`."),
			"scope":             optStr("Requested scope."),
			"audience":          optStr("Requested audience."),
			"issuer":            optStr("JWT issuer, for `client_assertion`."),
			"subject":           optStr("JWT subject, for `client_assertion`."),
			"signing_algorithm": optStr("JWT signing algorithm, e.g. `ES256` or `RS256`."),
			"expiry_time_unit": schema.StringAttribute{Optional: true,
				Description: "Unit of the token response expiry: `seconds`, `milliseconds`, `minutes` or `hours`.",
				Validators:  []validator.String{stringvalidator.OneOf("seconds", "milliseconds", "minutes", "hours")}},
			"signing_key": schema.StringAttribute{Optional: true, Sensitive: true, CustomType: jsontypes.NormalizedType{},
				Description: "JWK (as JSON, use `jsonencode()`) used to sign client assertions."},
			"field_mapping": fieldMappingAttr,
		},
	}
	endpointAuthAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Authentication Convoy performs against the endpoint. Set `type` and the matching nested object.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{Required: true, Description: "Authentication type: `api_key`, `basic_auth` or `oauth2`.",
				Validators: []validator.String{stringvalidator.OneOf("api_key", "basic_auth", "oauth2")}},
			"api_key":    headerKeyAttr,
			"basic_auth": basicAuthAttr,
			"oauth2":     oauth2Attr,
		},
	}
	mtlsAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Client certificate used for mutual TLS with the endpoint.",
		Attributes: map[string]schema.Attribute{
			"client_cert": schema.StringAttribute{Required: true, Description: "PEM encoded client certificate."},
			"client_key":  sensitiveStr("PEM encoded private key.", true),
		},
	}
)

func (r *EndpointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_endpoint"
}

func (r *EndpointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "An endpoint is a URL Convoy delivers webhook events to. In outgoing projects an endpoint usually " +
			"represents one of your customers' webhook receivers; in incoming projects it is your own service.",
		Attributes: map[string]schema.Attribute{
			"id":          idAttribute("endpoint"),
			"project_id":  projectIDAttribute(),
			"name":        schema.StringAttribute{Required: true, Description: "Endpoint name."},
			"url":         schema.StringAttribute{Required: true, Description: "URL events are delivered to."},
			"description": optStr("Human readable description."),
			"owner_id": schema.StringAttribute{Optional: true,
				Description: "Groups several endpoints together so a single event can be [fanned out](https://www.getconvoy.io/docs/product-manual/endpoints) to all of them. Convoy does not allow clearing an owner ID once set."},
			"support_email":       optStr("Email notified about endpoint state changes (e.g. disabled after failures). Requires a Convoy edition with advanced endpoint management."),
			"slack_webhook_url":   optStr("Slack incoming-webhook URL notified about endpoint failures."),
			"teams_webhook_url":   optStr("Microsoft Teams (Workflows) webhook URL notified about endpoint failures."),
			"http_timeout":        int64OC("Delivery HTTP timeout, in seconds."),
			"rate_limit":          int64OC("Maximum number of deliveries per `rate_limit_duration`."),
			"rate_limit_duration": int64OC("Rate limit window, in seconds."),
			"advanced_signatures": boolOC("Use Convoy's advanced (timestamped, multi-version) signature format. Outgoing projects only."),
			"content_type":        stringOC("`Content-Type` of delivered requests. Defaults to `application/json`."),
			"secret": schema.StringAttribute{
				Optional: true, Computed: true, Sensitive: true,
				Description: "Signing secret. Generated by Convoy when omitted. Changing it rotates the secret through Convoy's " +
					"expire-secret API (the previous secret stays valid for `secret_expiration`) instead of recreating the endpoint.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"secret_expiration": schema.Int64Attribute{
				Optional:    true,
				Description: "Grace period passed to Convoy's expire-secret API when `secret` changes, during which the previous secret remains valid. Defaults to `0`.",
			},
			"paused":           boolOC("Whether deliveries to the endpoint are paused. Toggled through Convoy's pause API."),
			"status":           schema.StringAttribute{Computed: true, Description: "Endpoint status: `active`, `inactive` (disabled after failures) or `paused`."},
			"authentication":   endpointAuthAttr,
			"mtls_client_cert": mtlsAttr,
			"created_at":       timestampAttribute("Creation timestamp."),
		},
	}
}

func (r *EndpointResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var auth types.Object
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("authentication"), &auth)...)
	var m endpointAuthModel
	if !objectAs(ctx, auth, &m, &resp.Diagnostics) || !shared.Known(m.Type) {
		return
	}
	want := map[string]types.Object{"api_key": m.APIKey, "basic_auth": m.BasicAuth, "oauth2": m.OAuth2}[m.Type.ValueString()]
	if want.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("authentication").AtName(m.Type.ValueString()), "Missing authentication settings",
			"authentication."+m.Type.ValueString()+" must be set when authentication.type is \""+m.Type.ValueString()+"\".")
	}
}

// ------------------------------------------------------------- conversions

func endpointAuthToAPI(ctx context.Context, o types.Object, diags *diag.Diagnostics) *convoy.EndpointAuthentication {
	var m endpointAuthModel
	if !objectAs(ctx, o, &m, diags) {
		return nil
	}
	out := &convoy.EndpointAuthentication{Type: m.Type.ValueString()}
	var hk headerKeyModel
	if objectAs(ctx, m.APIKey, &hk, diags) {
		out.APIKey = &convoy.HeaderKey{HeaderName: hk.HeaderName.ValueString(), HeaderValue: hk.HeaderValue.ValueString()}
	}
	var ba basicAuthModel
	if objectAs(ctx, m.BasicAuth, &ba, diags) {
		out.BasicAuth = &convoy.BasicAuth{Username: ba.Username.ValueString(), Password: ba.Password.ValueString()}
	}
	var o2 oauth2Model
	if objectAs(ctx, m.OAuth2, &o2, diags) {
		out.OAuth2 = &convoy.OAuth2{
			URL: o2.URL.ValueString(), ClientID: o2.ClientID.ValueString(), AuthenticationType: o2.AuthenticationType.ValueString(),
			ClientSecret: o2.ClientSecret.ValueString(), GrantType: o2.GrantType.ValueString(), Scope: o2.Scope.ValueString(),
			Audience: o2.Audience.ValueString(), Issuer: o2.Issuer.ValueString(), Subject: o2.Subject.ValueString(),
			SigningAlgorithm: o2.SigningAlgorithm.ValueString(), ExpiryTimeUnit: o2.ExpiryTimeUnit.ValueString(),
			SigningKey: shared.JSONRaw(o2.SigningKey),
		}
		var fm fieldMappingModel
		if objectAs(ctx, o2.FieldMapping, &fm, diags) {
			out.OAuth2.FieldMapping = &convoy.OAuth2FieldMapping{
				AccessToken: fm.AccessToken.ValueString(), TokenType: fm.TokenType.ValueString(), ExpiresIn: fm.ExpiresIn.ValueString(),
			}
		}
	}
	return out
}

func endpointAuthFromAPI(ctx context.Context, a *convoy.EndpointAuthentication, prior types.Object, diags *diag.Diagnostics) types.Object {
	attrs := objectAttrs(endpointAuthAttr)
	if a == nil || a.Type == "" {
		return types.ObjectNull(attrs)
	}
	var p endpointAuthModel
	objectAs(ctx, prior, &p, diags)

	m := endpointAuthModel{
		Type:      types.StringValue(a.Type),
		APIKey:    types.ObjectNull(objectAttrs(headerKeyAttr)),
		BasicAuth: types.ObjectNull(objectAttrs(basicAuthAttr)),
		OAuth2:    types.ObjectNull(objectAttrs(oauth2Attr)),
	}
	if a.APIKey != nil && a.APIKey.HeaderName != "" {
		var ph headerKeyModel
		objectAs(ctx, p.APIKey, &ph, diags)
		m.APIKey = objectFrom(ctx, objectAttrs(headerKeyAttr), headerKeyModel{
			HeaderName: types.StringValue(a.APIKey.HeaderName), HeaderValue: shared.KeepSensitive(ph.HeaderValue, a.APIKey.HeaderValue),
		}, diags)
	}
	if a.BasicAuth != nil && a.BasicAuth.Username != "" {
		var pb basicAuthModel
		objectAs(ctx, p.BasicAuth, &pb, diags)
		m.BasicAuth = objectFrom(ctx, objectAttrs(basicAuthAttr), basicAuthModel{
			Username: types.StringValue(a.BasicAuth.Username), Password: shared.KeepSensitive(pb.Password, a.BasicAuth.Password),
		}, diags)
	}
	if o := a.OAuth2; o != nil && o.URL != "" {
		var po oauth2Model
		objectAs(ctx, p.OAuth2, &po, diags)
		signingKey := po.SigningKey
		if !shared.Known(signingKey) {
			signingKey = shared.JSONFromAPI(jsontypes.NewNormalizedNull(), o.SigningKey)
		}
		fm := types.ObjectNull(objectAttrs(fieldMappingAttr))
		if f := o.FieldMapping; f != nil && (f.AccessToken != "" || f.TokenType != "" || f.ExpiresIn != "") {
			fm = objectFrom(ctx, objectAttrs(fieldMappingAttr), fieldMappingModel{
				AccessToken: shared.StrOrNull(f.AccessToken), TokenType: shared.StrOrNull(f.TokenType), ExpiresIn: shared.StrOrNull(f.ExpiresIn),
			}, diags)
		}
		m.OAuth2 = objectFrom(ctx, objectAttrs(oauth2Attr), oauth2Model{
			URL: types.StringValue(o.URL), ClientID: types.StringValue(o.ClientID), AuthenticationType: types.StringValue(o.AuthenticationType),
			ClientSecret: shared.KeepSensitive(po.ClientSecret, o.ClientSecret),
			GrantType:    keepPriorIfAPIEmpty(po.GrantType, o.GrantType), Scope: shared.StrOrNull(o.Scope),
			Audience: shared.StrOrNull(o.Audience), Issuer: shared.StrOrNull(o.Issuer), Subject: shared.StrOrNull(o.Subject),
			SigningAlgorithm: shared.StrOrNull(o.SigningAlgorithm), ExpiryTimeUnit: keepPriorIfAPIEmpty(po.ExpiryTimeUnit, o.ExpiryTimeUnit),
			SigningKey: signingKey, FieldMapping: fm,
		}, diags)
	}
	return objectFrom(ctx, attrs, m, diags)
}

// keepPriorIfAPIEmpty treats a value Convoy fills with a default as unchanged
// when the user did not configure it.
func keepPriorIfAPIEmpty(prior types.String, api string) types.String {
	if !shared.Known(prior) {
		return types.StringNull()
	}
	if api == "" {
		return prior
	}
	return types.StringValue(api)
}

func mtlsFromAPI(ctx context.Context, c *convoy.MtlsClientCert, prior types.Object, diags *diag.Diagnostics) types.Object {
	if c == nil || c.ClientCert == "" {
		return types.ObjectNull(objectAttrs(mtlsAttr))
	}
	var p mtlsModel
	objectAs(ctx, prior, &p, diags)
	return objectFrom(ctx, objectAttrs(mtlsAttr), mtlsModel{
		ClientCert: types.StringValue(c.ClientCert), ClientKey: shared.KeepSensitive(p.ClientKey, c.ClientKey),
	}, diags)
}

func (r *EndpointResource) request(ctx context.Context, m *EndpointResourceModel, prior *EndpointResourceModel, diags *diag.Diagnostics) convoy.EndpointRequest {
	req := convoy.EndpointRequest{
		Name: m.Name.ValueString(), URL: m.URL.ValueString(), Description: m.Description.ValueString(),
		OwnerID:            m.OwnerID.ValueString(),
		SupportEmail:       shared.OptStr(m.SupportEmail),
		SlackWebhookURL:    shared.OptStr(m.SlackWebhookURL),
		TeamsWebhookURL:    shared.OptStr(m.TeamsWebhookURL),
		HTTPTimeout:        m.HTTPTimeout.ValueInt64(),
		RateLimit:          m.RateLimit.ValueInt64(),
		RateLimitDuration:  m.RateLimitDuration.ValueInt64(),
		AdvancedSignatures: shared.OptBool(m.AdvancedSignatures),
		ContentType:        shared.OptStr(m.ContentType),
		Authentication:     endpointAuthToAPI(ctx, m.Authentication, diags),
	}
	// Explicitly clear notification channels removed from configuration.
	if prior != nil {
		empty := ""
		if req.SupportEmail == nil && shared.Known(prior.SupportEmail) {
			req.SupportEmail = &empty
		}
		if req.SlackWebhookURL == nil && shared.Known(prior.SlackWebhookURL) {
			req.SlackWebhookURL = &empty
		}
		if req.TeamsWebhookURL == nil && shared.Known(prior.TeamsWebhookURL) {
			req.TeamsWebhookURL = &empty
		}
	}
	var mt mtlsModel
	if objectAs(ctx, m.MtlsClientCert, &mt, diags) {
		req.MtlsClientCert = &convoy.MtlsClientCert{ClientCert: mt.ClientCert.ValueString(), ClientKey: mt.ClientKey.ValueString()}
	} else if prior != nil && shared.Known(prior.MtlsClientCert) {
		// Both fields empty removes the mTLS configuration.
		req.MtlsClientCert = &convoy.MtlsClientCert{}
	}
	return req
}

func (r *EndpointResource) apply(ctx context.Context, e *convoy.Endpoint, m *EndpointResourceModel, refresh bool, diags *diag.Diagnostics) {
	m.ID = types.StringValue(e.UID)
	if e.ProjectID != "" {
		m.ProjectID = types.StringValue(e.ProjectID)
	}
	merge(&m.Name, types.StringValue(e.Name), refresh)
	merge(&m.URL, types.StringValue(e.URL), refresh)
	merge(&m.Description, shared.StrOrNull(e.Description), refresh)
	merge(&m.OwnerID, shared.StrOrNull(e.OwnerID), refresh)
	merge(&m.SupportEmail, shared.StrOrNull(e.SupportEmail), refresh)
	merge(&m.SlackWebhookURL, shared.StrOrNull(e.SlackWebhookURL), refresh)
	merge(&m.TeamsWebhookURL, shared.StrOrNull(e.TeamsWebhookURL), refresh)
	merge(&m.HTTPTimeout, types.Int64Value(e.HTTPTimeout), refresh)
	merge(&m.RateLimit, types.Int64Value(e.RateLimit), refresh)
	merge(&m.RateLimitDuration, types.Int64Value(e.RateLimitDuration), refresh)
	merge(&m.AdvancedSignatures, types.BoolValue(e.AdvancedSignatures), refresh)
	merge(&m.ContentType, types.StringValue(e.ContentType), refresh)
	merge(&m.Secret, types.StringValue(e.ActiveSecret()), refresh)
	merge(&m.Paused, types.BoolValue(e.Status == "paused"), refresh)
	m.Status = types.StringValue(e.Status)
	merge(&m.Authentication, endpointAuthFromAPI(ctx, e.Authentication, m.Authentication, diags), refresh)
	merge(&m.MtlsClientCert, mtlsFromAPI(ctx, e.MtlsClientCert, m.MtlsClientCert, diags), refresh)
	m.CreatedAt = types.StringValue(e.CreatedAt)
}

// setPaused toggles the pause state if needed and returns the endpoint.
func (r *EndpointResource) setPaused(ctx context.Context, projectID string, e *convoy.Endpoint, want types.Bool) (*convoy.Endpoint, error) {
	if !shared.Known(want) || want.ValueBool() == (e.Status == "paused") {
		return e, nil
	}
	if !want.ValueBool() && e.Status != "paused" {
		return e, nil
	}
	if _, err := r.client().ToggleEndpointPause(ctx, projectID, e.UID); err != nil {
		return nil, err
	}
	return r.client().GetEndpoint(ctx, projectID, e.UID)
}

// ------------------------------------------------------------------- CRUD

func (r *EndpointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan EndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	body := r.request(ctx, &plan, nil, &resp.Diagnostics)
	if shared.Known(plan.Secret) {
		body.Secret = plan.Secret.ValueString()
	}
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client().CreateEndpoint(ctx, projectID, body)
	if err != nil {
		apiError(&resp.Diagnostics, "create endpoint", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	// Persist the ID right away so a failure below does not orphan the endpoint.
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), e.UID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)

	if e, err = r.client().GetEndpoint(ctx, projectID, e.UID); err == nil {
		e, err = r.setPaused(ctx, projectID, e, plan.Paused)
	}
	if err != nil {
		apiError(&resp.Diagnostics, "finish creating endpoint", err)
		return
	}
	r.apply(ctx, e, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EndpointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state EndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	e, err := r.client().GetEndpoint(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "endpoint") {
			return
		}
		apiError(&resp.Diagnostics, "read endpoint", err)
		return
	}
	r.apply(ctx, e, &state, true, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *EndpointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state EndpointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID, id := state.ProjectID.ValueString(), state.ID.ValueString()
	body := r.request(ctx, &plan, &state, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	if _, err := r.client().UpdateEndpoint(ctx, projectID, id, body); err != nil {
		apiError(&resp.Diagnostics, "update endpoint", err)
		return
	}
	if shared.Known(plan.Secret) && plan.Secret.ValueString() != state.Secret.ValueString() {
		_, err := r.client().ExpireEndpointSecret(ctx, projectID, id, convoy.ExpireSecretRequest{
			Secret: plan.Secret.ValueString(), Expiration: plan.SecretExpiration.ValueInt64(),
		})
		if err != nil {
			apiError(&resp.Diagnostics, "rotate endpoint secret", err)
			return
		}
	}
	e, err := r.client().GetEndpoint(ctx, projectID, id)
	if err == nil {
		e, err = r.setPaused(ctx, projectID, e, plan.Paused)
	}
	if err != nil {
		apiError(&resp.Diagnostics, "read endpoint after update", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.apply(ctx, e, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *EndpointResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state EndpointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client().DeleteEndpoint(ctx, state.ProjectID.ValueString(), state.ID.ValueString()); err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete endpoint", err)
	}
}

func (r *EndpointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, r.data, req, resp)
}

// importProjectScoped handles "<project_id>/<id>" (or "<id>" with a default
// provider project) import IDs.
func importProjectScoped(ctx context.Context, data *shared.ProviderData, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	def := ""
	if data != nil {
		def = data.ProjectID
	}
	parts, err := shared.ParseImportID(req.ID, 2, def)
	if err != nil {
		resp.Diagnostics.AddError("Invalid import ID", err.Error())
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), parts[0])...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
}
