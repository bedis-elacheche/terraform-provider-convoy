package resources

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/bedis-elacheche/terraform-provider-convoy/internal/convoy"
	"github.com/bedis-elacheche/terraform-provider-convoy/internal/provider/shared"
)

var (
	_ resource.Resource                   = &SourceResource{}
	_ resource.ResourceWithConfigure      = &SourceResource{}
	_ resource.ResourceWithImportState    = &SourceResource{}
	_ resource.ResourceWithValidateConfig = &SourceResource{}
)

func NewSourceResource() resource.Resource { return &SourceResource{} }

type SourceResource struct{ base }

type SourceResourceModel struct {
	ID                types.String `tfsdk:"id"`
	ProjectID         types.String `tfsdk:"project_id"`
	Name              types.String `tfsdk:"name"`
	Type              types.String `tfsdk:"type"`
	Provider          types.String `tfsdk:"source_provider"`
	IsDisabled        types.Bool   `tfsdk:"is_disabled"`
	Verifier          types.Object `tfsdk:"verifier"`
	PubSub            types.Object `tfsdk:"pub_sub"`
	CustomResponse    types.Object `tfsdk:"custom_response"`
	IdempotencyKeys   types.List   `tfsdk:"idempotency_keys"`
	ForwardHeaders    types.List   `tfsdk:"forward_headers"`
	EventTypeLocation types.String `tfsdk:"event_type_location"`
	BodyFunction      types.String `tfsdk:"body_function"`
	HeaderFunction    types.String `tfsdk:"header_function"`
	URL               types.String `tfsdk:"url"`
	MaskID            types.String `tfsdk:"mask_id"`
	CreatedAt         types.String `tfsdk:"created_at"`
}

type verifierModel struct {
	Type      types.String `tfsdk:"type"`
	HMac      types.Object `tfsdk:"hmac"`
	BasicAuth types.Object `tfsdk:"basic_auth"`
	APIKey    types.Object `tfsdk:"api_key"`
}

type hmacModel struct {
	Header   types.String `tfsdk:"header"`
	Hash     types.String `tfsdk:"hash"`
	Secret   types.String `tfsdk:"secret"`
	Encoding types.String `tfsdk:"encoding"`
}

type customResponseModel struct {
	Body        types.String `tfsdk:"body"`
	ContentType types.String `tfsdk:"content_type"`
}

type pubSubModel struct {
	Type    types.String `tfsdk:"type"`
	Workers types.Int64  `tfsdk:"workers"`
	SQS     types.Object `tfsdk:"sqs"`
	Google  types.Object `tfsdk:"google"`
	Kafka   types.Object `tfsdk:"kafka"`
	AMQP    types.Object `tfsdk:"amqp"`
}

type sqsModel struct {
	AccessKeyID   types.String `tfsdk:"access_key_id"`
	SecretKey     types.String `tfsdk:"secret_key"`
	DefaultRegion types.String `tfsdk:"default_region"`
	QueueName     types.String `tfsdk:"queue_name"`
	Endpoint      types.String `tfsdk:"endpoint"`
}

type googleModel struct {
	ProjectID      types.String `tfsdk:"project_id"`
	SubscriptionID types.String `tfsdk:"subscription_id"`
	ServiceAccount types.String `tfsdk:"service_account"`
}

type kafkaModel struct {
	Brokers         types.List   `tfsdk:"brokers"`
	ConsumerGroupID types.String `tfsdk:"consumer_group_id"`
	TopicName       types.String `tfsdk:"topic_name"`
	Auth            types.Object `tfsdk:"auth"`
}

type kafkaAuthModel struct {
	Type     types.String `tfsdk:"type"`
	Hash     types.String `tfsdk:"hash"`
	Username types.String `tfsdk:"username"`
	Password types.String `tfsdk:"password"`
	TLS      types.Bool   `tfsdk:"tls"`
}

type amqpModel struct {
	Schema             types.String `tfsdk:"schema"`
	Host               types.String `tfsdk:"host"`
	Port               types.String `tfsdk:"port"`
	Queue              types.String `tfsdk:"queue"`
	Vhost              types.String `tfsdk:"vhost"`
	DeadLetterExchange types.String `tfsdk:"dead_letter_exchange"`
	Username           types.String `tfsdk:"username"`
	Password           types.String `tfsdk:"password"`
	BindExchange       types.String `tfsdk:"bind_exchange"`
	RoutingKey         types.String `tfsdk:"routing_key"`
}

var (
	hmacAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "HMAC signature verification.",
		Attributes: map[string]schema.Attribute{
			"header": schema.StringAttribute{Required: true, Description: "Header carrying the signature, e.g. `X-Hub-Signature-256`."},
			"hash":   schema.StringAttribute{Required: true, Description: "Hash function, e.g. `SHA256` or `SHA512`."},
			"secret": sensitiveStr("Shared HMAC secret.", true),
			"encoding": schema.StringAttribute{Required: true, Description: "Signature encoding, `hex` or `base64`.",
				Validators: []validator.String{stringvalidator.OneOf("hex", "base64")}},
		},
	}
	verifierAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "How Convoy verifies requests ingested by an `http` source. Set `type` and the matching nested object.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{Required: true, Description: "Verifier type: `noop`, `hmac`, `basic_auth` or `api_key`.",
				Validators: []validator.String{stringvalidator.OneOf("noop", "hmac", "basic_auth", "api_key")}},
			"hmac":       hmacAttr,
			"basic_auth": basicAuthAttr,
			"api_key":    headerKeyAttr,
		},
	}
	customResponseAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Custom HTTP response returned to the webhook sender.",
		Attributes: map[string]schema.Attribute{
			"body":         schema.StringAttribute{Required: true, Description: "Response body."},
			"content_type": schema.StringAttribute{Required: true, Description: "Response `Content-Type`."},
		},
	}
	sqsAttr = schema.SingleNestedAttribute{
		Optional: true, Description: "Amazon SQS settings.",
		Attributes: map[string]schema.Attribute{
			"access_key_id":  schema.StringAttribute{Required: true, Description: "AWS access key ID."},
			"secret_key":     sensitiveStr("AWS secret access key.", true),
			"default_region": schema.StringAttribute{Required: true, Description: "AWS region."},
			"queue_name":     schema.StringAttribute{Required: true, Description: "Queue name."},
			"endpoint":       optStr("Custom SQS endpoint (e.g. LocalStack)."),
		},
	}
	googleAttr = schema.SingleNestedAttribute{
		Optional: true, Description: "Google Cloud Pub/Sub settings.",
		Attributes: map[string]schema.Attribute{
			"project_id":      schema.StringAttribute{Required: true, Description: "GCP project ID."},
			"subscription_id": schema.StringAttribute{Required: true, Description: "Pub/Sub subscription ID."},
			"service_account": sensitiveStr("Base64 encoded service account JSON key, e.g. `filebase64(\"sa.json\")`.", true),
		},
	}
	kafkaAuthAttr = schema.SingleNestedAttribute{
		Optional: true, Description: "Kafka authentication.",
		Attributes: map[string]schema.Attribute{
			"type":     optStr("Authentication type, `plain` or `scram`."),
			"hash":     optStr("SCRAM hash, `SHA256` or `SHA512`."),
			"username": optStr("Username."),
			"password": sensitiveStr("Password.", false),
			"tls":      schema.BoolAttribute{Optional: true, Description: "Connect over TLS."},
		},
	}
	kafkaAttr = schema.SingleNestedAttribute{
		Optional: true, Description: "Apache Kafka settings.",
		Attributes: map[string]schema.Attribute{
			"brokers":           schema.ListAttribute{Required: true, ElementType: types.StringType, Description: "Broker addresses."},
			"consumer_group_id": optStr("Consumer group ID."),
			"topic_name":        schema.StringAttribute{Required: true, Description: "Topic name."},
			"auth":              kafkaAuthAttr,
		},
	}
	amqpAttr = schema.SingleNestedAttribute{
		Optional: true, Description: "RabbitMQ / AMQP settings.",
		Attributes: map[string]schema.Attribute{
			"schema":               schema.StringAttribute{Required: true, Description: "URI scheme, `amqp` or `amqps`."},
			"host":                 schema.StringAttribute{Required: true, Description: "Broker host."},
			"port":                 schema.StringAttribute{Required: true, Description: "Broker port."},
			"queue":                schema.StringAttribute{Required: true, Description: "Queue name."},
			"vhost":                optStr("Virtual host."),
			"dead_letter_exchange": optStr("Dead letter exchange."),
			"username":             optStr("Username."),
			"password":             sensitiveStr("Password.", false),
			"bind_exchange":        optStr("Exchange the queue is bound to."),
			"routing_key":          optStr("Routing key of the binding."),
		},
	}
	pubSubAttr = schema.SingleNestedAttribute{
		Optional:    true,
		Description: "Message broker settings for `pub_sub` sources. Set `type` and the matching nested object.",
		Attributes: map[string]schema.Attribute{
			"type": schema.StringAttribute{Required: true, Description: "Broker type: `sqs`, `google`, `kafka` or `amqp`.",
				Validators: []validator.String{stringvalidator.OneOf("sqs", "google", "kafka", "amqp")}},
			"workers": schema.Int64Attribute{Required: true, Description: "Number of consumer workers."},
			"sqs":     sqsAttr,
			"google":  googleAttr,
			"kafka":   kafkaAttr,
			"amqp":    amqpAttr,
		},
	}
)

func (r *SourceResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_source"
}

func (r *SourceResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A source is where events enter Convoy: an HTTP ingest URL for incoming projects (optionally verified), " +
			"or a message broker (SQS, Google Pub/Sub, Kafka, RabbitMQ) for outgoing projects.",
		Attributes: map[string]schema.Attribute{
			"id":         idAttribute("source"),
			"project_id": projectIDAttribute(),
			"name":       schema.StringAttribute{Required: true, Description: "Source name."},
			"type": schema.StringAttribute{Required: true,
				Description: "Source type: `http` (ingest URL, incoming projects), `pub_sub` (message broker), `rest_api` or `db_change_stream`.",
				Validators:  []validator.String{stringvalidator.OneOf("http", "rest_api", "pub_sub", "db_change_stream")}},
			"source_provider": schema.StringAttribute{Optional: true,
				Description: "Pre-configured provider verification: `github`, `twitter` or `shopify`.",
				Validators:  []validator.String{stringvalidator.OneOf("github", "twitter", "shopify")}},
			"is_disabled":         boolOC("Whether the source is disabled."),
			"verifier":            verifierAttr,
			"pub_sub":             pubSubAttr,
			"custom_response":     customResponseAttr,
			"idempotency_keys":    schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Request locations used to de-duplicate events, e.g. `request.headers.X-Request-Id` or `request.body.id`."},
			"forward_headers":     schema.ListAttribute{Optional: true, ElementType: types.StringType, Description: "Ingest request headers forwarded to endpoints."},
			"event_type_location": optStr("Where to read the event type from in ingested requests, e.g. `request.body.type`."),
			"body_function":       optStr("JavaScript `transform` function applied to ingested payloads."),
			"header_function":     optStr("JavaScript `transform` function applied to ingested headers."),
			"url":                 schema.StringAttribute{Computed: true, Description: "Ingest URL to give to the webhook provider (`http` sources)."},
			"mask_id":             schema.StringAttribute{Computed: true, Description: "Public identifier used in the ingest URL."},
			"created_at":          timestampAttribute("Creation timestamp."),
		},
	}
}

func (r *SourceResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var m SourceResourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &m)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var v verifierModel
	if objectAs(ctx, m.Verifier, &v, &resp.Diagnostics) && shared.Known(v.Type) {
		nested := map[string]types.Object{"hmac": v.HMac, "basic_auth": v.BasicAuth, "api_key": v.APIKey}
		if o, ok := nested[v.Type.ValueString()]; ok && o.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("verifier").AtName(v.Type.ValueString()), "Missing verifier settings",
				"verifier."+v.Type.ValueString()+" must be set when verifier.type is \""+v.Type.ValueString()+"\".")
		}
	}
	var ps pubSubModel
	if objectAs(ctx, m.PubSub, &ps, &resp.Diagnostics) && shared.Known(ps.Type) {
		nested := map[string]types.Object{"sqs": ps.SQS, "google": ps.Google, "kafka": ps.Kafka, "amqp": ps.AMQP}
		if o := nested[ps.Type.ValueString()]; o.IsNull() {
			resp.Diagnostics.AddAttributeError(path.Root("pub_sub").AtName(ps.Type.ValueString()), "Missing pub_sub settings",
				"pub_sub."+ps.Type.ValueString()+" must be set when pub_sub.type is \""+ps.Type.ValueString()+"\".")
		}
	}
	if shared.Known(m.Type) && m.Type.ValueString() == "pub_sub" && m.PubSub.IsNull() {
		resp.Diagnostics.AddAttributeError(path.Root("pub_sub"), "Missing pub_sub settings", "pub_sub must be set for sources of type \"pub_sub\".")
	}
}

// ------------------------------------------------------------- conversions

func verifierToAPI(ctx context.Context, o types.Object, diags *diag.Diagnostics) *convoy.Verifier {
	var m verifierModel
	if !objectAs(ctx, o, &m, diags) {
		return nil
	}
	out := &convoy.Verifier{Type: m.Type.ValueString()}
	var h hmacModel
	if objectAs(ctx, m.HMac, &h, diags) {
		out.HMac = &convoy.HMac{Header: h.Header.ValueString(), Hash: h.Hash.ValueString(), Secret: h.Secret.ValueString(), Encoding: h.Encoding.ValueString()}
	}
	var b basicAuthModel
	if objectAs(ctx, m.BasicAuth, &b, diags) {
		out.BasicAuth = &convoy.BasicAuth{Username: b.Username.ValueString(), Password: b.Password.ValueString()}
	}
	var k headerKeyModel
	if objectAs(ctx, m.APIKey, &k, diags) {
		out.APIKey = &convoy.HeaderKey{HeaderName: k.HeaderName.ValueString(), HeaderValue: k.HeaderValue.ValueString()}
	}
	return out
}

func verifierFromAPI(ctx context.Context, v *convoy.Verifier, prior types.Object, diags *diag.Diagnostics) types.Object {
	attrs := objectAttrs(verifierAttr)
	if v == nil || v.Type == "" || (v.Type == "noop" && prior.IsNull()) {
		return types.ObjectNull(attrs)
	}
	var p verifierModel
	objectAs(ctx, prior, &p, diags)
	m := verifierModel{
		Type:      types.StringValue(v.Type),
		HMac:      types.ObjectNull(objectAttrs(hmacAttr)),
		BasicAuth: types.ObjectNull(objectAttrs(basicAuthAttr)),
		APIKey:    types.ObjectNull(objectAttrs(headerKeyAttr)),
	}
	if h := v.HMac; h != nil && h.Header != "" {
		var ph hmacModel
		objectAs(ctx, p.HMac, &ph, diags)
		m.HMac = objectFrom(ctx, objectAttrs(hmacAttr), hmacModel{
			Header: types.StringValue(h.Header), Hash: types.StringValue(h.Hash), Encoding: types.StringValue(h.Encoding),
			Secret: shared.KeepSensitive(ph.Secret, h.Secret),
		}, diags)
	}
	if b := v.BasicAuth; b != nil && b.Username != "" {
		var pb basicAuthModel
		objectAs(ctx, p.BasicAuth, &pb, diags)
		m.BasicAuth = objectFrom(ctx, objectAttrs(basicAuthAttr), basicAuthModel{
			Username: types.StringValue(b.Username), Password: shared.KeepSensitive(pb.Password, b.Password),
		}, diags)
	}
	if k := v.APIKey; k != nil && k.HeaderName != "" {
		var pk headerKeyModel
		objectAs(ctx, p.APIKey, &pk, diags)
		m.APIKey = objectFrom(ctx, objectAttrs(headerKeyAttr), headerKeyModel{
			HeaderName: types.StringValue(k.HeaderName), HeaderValue: shared.KeepSensitive(pk.HeaderValue, k.HeaderValue),
		}, diags)
	}
	return objectFrom(ctx, attrs, m, diags)
}

func pubSubToAPI(ctx context.Context, o types.Object, diags *diag.Diagnostics) *convoy.PubSubConfig {
	var m pubSubModel
	if !objectAs(ctx, o, &m, diags) {
		return nil
	}
	out := &convoy.PubSubConfig{Type: m.Type.ValueString(), Workers: m.Workers.ValueInt64()}
	var s sqsModel
	if objectAs(ctx, m.SQS, &s, diags) {
		out.SQS = &convoy.SQSPubSub{AccessKeyID: s.AccessKeyID.ValueString(), SecretKey: s.SecretKey.ValueString(),
			DefaultRegion: s.DefaultRegion.ValueString(), QueueName: s.QueueName.ValueString(), Endpoint: s.Endpoint.ValueString()}
	}
	var g googleModel
	if objectAs(ctx, m.Google, &g, diags) {
		out.Google = &convoy.GooglePubSub{ProjectID: g.ProjectID.ValueString(), SubscriptionID: g.SubscriptionID.ValueString(), ServiceAccount: g.ServiceAccount.ValueString()}
	}
	var k kafkaModel
	if objectAs(ctx, m.Kafka, &k, diags) {
		out.Kafka = &convoy.KafkaPubSub{Brokers: shared.StringList(ctx, k.Brokers, diags), ConsumerGroupID: k.ConsumerGroupID.ValueString(), TopicName: k.TopicName.ValueString()}
		var ka kafkaAuthModel
		if objectAs(ctx, k.Auth, &ka, diags) {
			out.Kafka.Auth = &convoy.KafkaAuth{Type: ka.Type.ValueString(), Hash: ka.Hash.ValueString(), Username: ka.Username.ValueString(),
				Password: ka.Password.ValueString(), TLS: ka.TLS.ValueBool()}
		}
	}
	var a amqpModel
	if objectAs(ctx, m.AMQP, &a, diags) {
		out.AMQP = &convoy.AMQPPubSub{Schema: a.Schema.ValueString(), Host: a.Host.ValueString(), Port: a.Port.ValueString(),
			Queue: a.Queue.ValueString(), Vhost: a.Vhost.ValueString(), DeadLetterExchange: a.DeadLetterExchange.ValueString()}
		if shared.Known(a.Username) || shared.Known(a.Password) {
			out.AMQP.Auth = &convoy.AMQPAuth{User: a.Username.ValueString(), Password: a.Password.ValueString()}
		}
		if shared.Known(a.BindExchange) || shared.Known(a.RoutingKey) {
			out.AMQP.BindExchange = &convoy.AMQPExchange{Exchange: a.BindExchange.ValueString(), RoutingKey: a.RoutingKey.ValueString()}
		}
	}
	return out
}

func pubSubFromAPI(ctx context.Context, ps *convoy.PubSubConfig, prior types.Object, diags *diag.Diagnostics) types.Object {
	attrs := objectAttrs(pubSubAttr)
	if ps == nil || ps.Type == "" {
		return types.ObjectNull(attrs)
	}
	var p pubSubModel
	objectAs(ctx, prior, &p, diags)
	m := pubSubModel{
		Type: types.StringValue(ps.Type), Workers: types.Int64Value(ps.Workers),
		SQS: types.ObjectNull(objectAttrs(sqsAttr)), Google: types.ObjectNull(objectAttrs(googleAttr)),
		Kafka: types.ObjectNull(objectAttrs(kafkaAttr)), AMQP: types.ObjectNull(objectAttrs(amqpAttr)),
	}
	if s := ps.SQS; s != nil && s.QueueName != "" {
		var pp sqsModel
		objectAs(ctx, p.SQS, &pp, diags)
		m.SQS = objectFrom(ctx, objectAttrs(sqsAttr), sqsModel{
			AccessKeyID: types.StringValue(s.AccessKeyID), SecretKey: shared.KeepSensitive(pp.SecretKey, s.SecretKey),
			DefaultRegion: types.StringValue(s.DefaultRegion), QueueName: types.StringValue(s.QueueName), Endpoint: shared.StrOrNull(s.Endpoint),
		}, diags)
	}
	if g := ps.Google; g != nil && g.ProjectID != "" {
		var pp googleModel
		objectAs(ctx, p.Google, &pp, diags)
		m.Google = objectFrom(ctx, objectAttrs(googleAttr), googleModel{
			ProjectID: types.StringValue(g.ProjectID), SubscriptionID: types.StringValue(g.SubscriptionID),
			ServiceAccount: shared.KeepSensitive(pp.ServiceAccount, g.ServiceAccount),
		}, diags)
	}
	if k := ps.Kafka; k != nil && k.TopicName != "" {
		var pp kafkaModel
		objectAs(ctx, p.Kafka, &pp, diags)
		auth := types.ObjectNull(objectAttrs(kafkaAuthAttr))
		if ka := k.Auth; ka != nil && (ka.Type != "" || ka.Username != "" || ka.TLS) {
			var pa kafkaAuthModel
			objectAs(ctx, pp.Auth, &pa, diags)
			tls := types.BoolValue(ka.TLS)
			if !ka.TLS && pa.TLS.IsNull() {
				tls = types.BoolNull()
			}
			auth = objectFrom(ctx, objectAttrs(kafkaAuthAttr), kafkaAuthModel{
				Type: shared.StrOrNull(ka.Type), Hash: shared.StrOrNull(ka.Hash), Username: shared.StrOrNull(ka.Username),
				Password: shared.KeepSensitive(pa.Password, ka.Password), TLS: tls,
			}, diags)
		}
		m.Kafka = objectFrom(ctx, objectAttrs(kafkaAttr), kafkaModel{
			Brokers: shared.ListOfStrings(k.Brokers), ConsumerGroupID: shared.StrOrNull(k.ConsumerGroupID),
			TopicName: types.StringValue(k.TopicName), Auth: auth,
		}, diags)
	}
	if a := ps.AMQP; a != nil && a.Host != "" {
		var pp amqpModel
		objectAs(ctx, p.AMQP, &pp, diags)
		am := amqpModel{
			Schema: types.StringValue(a.Schema), Host: types.StringValue(a.Host), Port: types.StringValue(a.Port),
			Queue: types.StringValue(a.Queue), Vhost: shared.StrOrNull(a.Vhost), DeadLetterExchange: shared.StrOrNull(a.DeadLetterExchange),
			Username: types.StringNull(), Password: pp.Password, BindExchange: types.StringNull(), RoutingKey: types.StringNull(),
		}
		if a.Auth != nil {
			am.Username = shared.StrOrNull(a.Auth.User)
			am.Password = shared.KeepSensitive(pp.Password, a.Auth.Password)
		}
		if a.BindExchange != nil {
			am.BindExchange, am.RoutingKey = shared.StrOrNull(a.BindExchange.Exchange), shared.StrOrNull(a.BindExchange.RoutingKey)
		}
		m.AMQP = objectFrom(ctx, objectAttrs(amqpAttr), am, diags)
	}
	return objectFrom(ctx, attrs, m, diags)
}

func customResponseFromAPI(ctx context.Context, c *convoy.CustomResponse, diags *diag.Diagnostics) types.Object {
	if c == nil || (c.Body == "" && c.ContentType == "") {
		return types.ObjectNull(objectAttrs(customResponseAttr))
	}
	return objectFrom(ctx, objectAttrs(customResponseAttr), customResponseModel{
		Body: types.StringValue(c.Body), ContentType: types.StringValue(c.ContentType)}, diags)
}

func optList(in []string) types.List {
	if len(in) == 0 {
		return types.ListNull(types.StringType)
	}
	return shared.ListOfStrings(in)
}

func (r *SourceResource) request(ctx context.Context, m *SourceResourceModel, prior *SourceResourceModel, diags *diag.Diagnostics) convoy.SourceRequest {
	req := convoy.SourceRequest{
		Name: m.Name.ValueString(), Type: m.Type.ValueString(), Provider: m.Provider.ValueString(),
		IsDisabled:        shared.OptBool(m.IsDisabled),
		Verifier:          verifierToAPI(ctx, m.Verifier, diags),
		PubSub:            pubSubToAPI(ctx, m.PubSub, diags),
		IdempotencyKeys:   shared.StringList(ctx, m.IdempotencyKeys, diags),
		ForwardHeaders:    shared.StringList(ctx, m.ForwardHeaders, diags),
		EventTypeLocation: shared.OptStr(m.EventTypeLocation),
		BodyFunction:      shared.OptStr(m.BodyFunction),
		HeaderFunction:    shared.OptStr(m.HeaderFunction),
	}
	if req.Verifier == nil && req.Type == "http" {
		// Convoy requires a verifier on update; noop means "accept everything".
		req.Verifier = &convoy.Verifier{Type: "noop"}
	}
	var cr customResponseModel
	if objectAs(ctx, m.CustomResponse, &cr, diags) {
		req.CustomResponse = &convoy.CustomResponse{Body: cr.Body.ValueString(), ContentType: cr.ContentType.ValueString()}
	}
	if prior != nil {
		empty := ""
		if req.CustomResponse == nil && shared.Known(prior.CustomResponse) {
			req.CustomResponse = &convoy.CustomResponse{}
		}
		if req.ForwardHeaders == nil && shared.Known(prior.ForwardHeaders) {
			req.ForwardHeaders = []string{}
		}
		if req.EventTypeLocation == nil && shared.Known(prior.EventTypeLocation) {
			req.EventTypeLocation = &empty
		}
		if req.BodyFunction == nil && shared.Known(prior.BodyFunction) {
			req.BodyFunction = &empty
		}
		if req.HeaderFunction == nil && shared.Known(prior.HeaderFunction) {
			req.HeaderFunction = &empty
		}
	}
	if req.IdempotencyKeys == nil {
		req.IdempotencyKeys = []string{}
	}
	return req
}

func (r *SourceResource) apply(ctx context.Context, s *convoy.Source, m *SourceResourceModel, refresh bool, diags *diag.Diagnostics) {
	m.ID = types.StringValue(s.UID)
	if s.ProjectID != "" {
		m.ProjectID = types.StringValue(s.ProjectID)
	}
	merge(&m.Name, types.StringValue(s.Name), refresh)
	merge(&m.Type, types.StringValue(s.Type), refresh)
	merge(&m.Provider, shared.StrOrNull(s.Provider), refresh)
	merge(&m.IsDisabled, types.BoolValue(s.IsDisabled), refresh)
	merge(&m.Verifier, verifierFromAPI(ctx, s.Verifier, m.Verifier, diags), refresh)
	merge(&m.PubSub, pubSubFromAPI(ctx, s.PubSub, m.PubSub, diags), refresh)
	merge(&m.CustomResponse, customResponseFromAPI(ctx, s.CustomResponse, diags), refresh)
	merge(&m.IdempotencyKeys, optList(s.IdempotencyKeys), refresh)
	merge(&m.ForwardHeaders, optList(s.ForwardHeaders), refresh)
	merge(&m.EventTypeLocation, shared.StrOrNull(s.EventTypeLocation), refresh)
	merge(&m.BodyFunction, shared.StrPtrOrNull(emptyToNil(s.BodyFunction)), refresh)
	merge(&m.HeaderFunction, shared.StrPtrOrNull(emptyToNil(s.HeaderFunction)), refresh)
	m.URL = shared.StrOrNull(s.URL)
	m.MaskID = shared.StrOrNull(s.MaskID)
	m.CreatedAt = types.StringValue(s.CreatedAt)
}

func emptyToNil(s *string) *string {
	if s == nil || *s == "" {
		return nil
	}
	return s
}

// ------------------------------------------------------------------- CRUD

func (r *SourceResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan SourceResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	projectID := r.data.ResolveProjectID(plan.ProjectID, &resp.Diagnostics)
	body := r.request(ctx, &plan, nil, &resp.Diagnostics)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client().CreateSource(ctx, projectID, body)
	if err != nil {
		apiError(&resp.Diagnostics, "create source", err)
		return
	}
	plan.ProjectID = types.StringValue(projectID)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), s.UID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("project_id"), projectID)...)

	// is_disabled is not accepted on create; apply it with an update.
	if shared.Known(plan.IsDisabled) && plan.IsDisabled.ValueBool() != s.IsDisabled {
		if _, err := r.client().UpdateSource(ctx, projectID, s.UID, body); err != nil {
			apiError(&resp.Diagnostics, "disable source", err)
			return
		}
	}
	if s, err = r.client().GetSource(ctx, projectID, s.UID); err != nil {
		apiError(&resp.Diagnostics, "read source after create", err)
		return
	}
	r.apply(ctx, s, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SourceResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state SourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.client().GetSource(ctx, state.ProjectID.ValueString(), state.ID.ValueString())
	if err != nil {
		if handleNotFound(ctx, err, resp, "source") {
			return
		}
		apiError(&resp.Diagnostics, "read source", err)
		return
	}
	r.apply(ctx, s, &state, true, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *SourceResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state SourceResourceModel
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
	if _, err := r.client().UpdateSource(ctx, projectID, id, body); err != nil {
		apiError(&resp.Diagnostics, "update source", err)
		return
	}
	s, err := r.client().GetSource(ctx, projectID, id)
	if err != nil {
		apiError(&resp.Diagnostics, "read source after update", err)
		return
	}
	plan.ID, plan.ProjectID = state.ID, state.ProjectID
	r.apply(ctx, s, &plan, false, &resp.Diagnostics)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *SourceResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state SourceResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.client().DeleteSource(ctx, state.ProjectID.ValueString(), state.ID.ValueString()); err != nil && !convoy.IsNotFound(err) {
		apiError(&resp.Diagnostics, "delete source", err)
	}
}

func (r *SourceResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	importProjectScoped(ctx, r.data, req, resp)
}
