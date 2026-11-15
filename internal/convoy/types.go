package convoy

import "encoding/json"

// ---------------------------------------------------------------- projects

type Project struct {
	UID            string         `json:"uid"`
	Name           string         `json:"name"`
	LogoURL        string         `json:"logo_url"`
	OrganisationID string         `json:"organisation_id"`
	Type           string         `json:"type"`
	Config         *ProjectConfig `json:"config,omitempty"`
	RetainedEvents int64          `json:"retained_events"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type ProjectConfig struct {
	MaxPayloadReadSize             *int64                `json:"max_payload_read_size,omitempty"`
	ReplayAttacksPreventionEnabled *bool                 `json:"replay_attacks_prevention_enabled,omitempty"`
	AddEventIDTraceHeaders         *bool                 `json:"add_event_id_trace_headers,omitempty"`
	DisableEndpoint                *bool                 `json:"disable_endpoint,omitempty"`
	MultipleEndpointSubscriptions  *bool                 `json:"multiple_endpoint_subscriptions,omitempty"`
	SearchPolicy                   *string               `json:"search_policy,omitempty"`
	SSL                            *SSLConfig            `json:"ssl,omitempty"`
	RateLimit                      *RateLimitConfig      `json:"ratelimit,omitempty"`
	Strategy                       *StrategyConfig       `json:"strategy,omitempty"`
	Signature                      *SignatureConfig      `json:"signature,omitempty"`
	MetaEvent                      *MetaEventConfig      `json:"meta_event,omitempty"`
	CircuitBreaker                 *CircuitBreakerConfig `json:"circuit_breaker,omitempty"`
}

type SSLConfig struct {
	EnforceSecureEndpoints bool `json:"enforce_secure_endpoints"`
}

type RateLimitConfig struct {
	Count    int64 `json:"count"`
	Duration int64 `json:"duration"`
}

type StrategyConfig struct {
	Type       string `json:"type"`
	Duration   int64  `json:"duration"`
	RetryCount int64  `json:"retry_count"`
}

type SignatureConfig struct {
	Header   string             `json:"header,omitempty"`
	Versions []SignatureVersion `json:"versions"`
}

type SignatureVersion struct {
	UID       string `json:"uid,omitempty"`
	Hash      string `json:"hash"`
	Encoding  string `json:"encoding"`
	CreatedAt string `json:"created_at,omitempty"`
}

type MetaEventConfig struct {
	IsEnabled bool     `json:"is_enabled"`
	Type      string   `json:"type,omitempty"`
	EventType []string `json:"event_type"`
	URL       string   `json:"url,omitempty"`
	Secret    string   `json:"secret,omitempty"`
}

type CircuitBreakerConfig struct {
	SampleRate                  int64 `json:"sample_rate"`
	ErrorTimeout                int64 `json:"error_timeout"`
	FailureThreshold            int64 `json:"failure_threshold"`
	SuccessThreshold            int64 `json:"success_threshold"`
	ObservabilityWindow         int64 `json:"observability_window"`
	MinimumRequestCount         int64 `json:"minimum_request_count"`
	ConsecutiveFailureThreshold int64 `json:"consecutive_failure_threshold"`
}

type CreateProjectRequest struct {
	Name    string         `json:"name"`
	Type    string         `json:"type"`
	LogoURL string         `json:"logo_url,omitempty"`
	Config  *ProjectConfig `json:"config,omitempty"`
}

type UpdateProjectRequest struct {
	Name    string         `json:"name"`
	LogoURL string         `json:"logo_url"`
	Config  *ProjectConfig `json:"config,omitempty"`
}

type APIKey struct {
	UID       string `json:"uid"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	KeyType   string `json:"key_type"`
	ExpiresAt string `json:"expires_at"`
}

type CreateProjectResponse struct {
	Project *Project `json:"project"`
	APIKey  *APIKey  `json:"api_key"`
}

// --------------------------------------------------------------- endpoints

type Endpoint struct {
	UID                string                  `json:"uid"`
	ProjectID          string                  `json:"project_id"`
	Name               string                  `json:"name"`
	URL                string                  `json:"url"`
	Description        string                  `json:"description"`
	OwnerID            string                  `json:"owner_id"`
	Status             string                  `json:"status"`
	SupportEmail       string                  `json:"support_email"`
	SlackWebhookURL    string                  `json:"slack_webhook_url"`
	TeamsWebhookURL    string                  `json:"teams_webhook_url"`
	HTTPTimeout        int64                   `json:"http_timeout"`
	RateLimit          int64                   `json:"rate_limit"`
	RateLimitDuration  int64                   `json:"rate_limit_duration"`
	AdvancedSignatures bool                    `json:"advanced_signatures"`
	ContentType        string                  `json:"content_type"`
	Secrets            []EndpointSecret        `json:"secrets"`
	Authentication     *EndpointAuthentication `json:"authentication,omitempty"`
	MtlsClientCert     *MtlsClientCert         `json:"mtls_client_cert,omitempty"`
	CreatedAt          string                  `json:"created_at"`
	UpdatedAt          string                  `json:"updated_at"`
}

// ActiveSecret returns the newest secret that has no expiry set.
func (e *Endpoint) ActiveSecret() string {
	for i := len(e.Secrets) - 1; i >= 0; i-- {
		if e.Secrets[i].ExpiresAt == "" && e.Secrets[i].DeletedAt == "" {
			return e.Secrets[i].Value
		}
	}
	if n := len(e.Secrets); n > 0 {
		return e.Secrets[n-1].Value
	}
	return ""
}

type EndpointSecret struct {
	UID       string `json:"uid"`
	Value     string `json:"value"`
	ExpiresAt string `json:"expires_at,omitempty"`
	DeletedAt string `json:"deleted_at,omitempty"`
	CreatedAt string `json:"created_at,omitempty"`
}

type EndpointAuthentication struct {
	Type      string     `json:"type"`
	APIKey    *HeaderKey `json:"api_key,omitempty"`
	BasicAuth *BasicAuth `json:"basic_auth,omitempty"`
	OAuth2    *OAuth2    `json:"oauth2,omitempty"`
}

type HeaderKey struct {
	HeaderName  string `json:"header_name"`
	HeaderValue string `json:"header_value"`
}

type BasicAuth struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type OAuth2 struct {
	URL                string              `json:"url"`
	ClientID           string              `json:"client_id"`
	AuthenticationType string              `json:"authentication_type"`
	ClientSecret       string              `json:"client_secret,omitempty"`
	GrantType          string              `json:"grant_type,omitempty"`
	Scope              string              `json:"scope,omitempty"`
	Audience           string              `json:"audience,omitempty"`
	Issuer             string              `json:"issuer,omitempty"`
	Subject            string              `json:"subject,omitempty"`
	SigningAlgorithm   string              `json:"signing_algorithm,omitempty"`
	ExpiryTimeUnit     string              `json:"expiry_time_unit,omitempty"`
	SigningKey         json.RawMessage     `json:"signing_key,omitempty"`
	FieldMapping       *OAuth2FieldMapping `json:"field_mapping,omitempty"`
}

type OAuth2FieldMapping struct {
	AccessToken string `json:"access_token,omitempty"`
	TokenType   string `json:"token_type,omitempty"`
	ExpiresIn   string `json:"expires_in,omitempty"`
}

type MtlsClientCert struct {
	ClientCert string `json:"client_cert"`
	ClientKey  string `json:"client_key"`
}

// EndpointRequest is the body of both create and update; Convoy's update is
// a full replacement of the mutable fields.
type EndpointRequest struct {
	Name               string                  `json:"name"`
	URL                string                  `json:"url"`
	Description        string                  `json:"description"`
	OwnerID            string                  `json:"owner_id,omitempty"`
	Secret             string                  `json:"secret,omitempty"`
	SupportEmail       *string                 `json:"support_email,omitempty"`
	SlackWebhookURL    *string                 `json:"slack_webhook_url,omitempty"`
	TeamsWebhookURL    *string                 `json:"teams_webhook_url,omitempty"`
	HTTPTimeout        int64                   `json:"http_timeout,omitempty"`
	RateLimit          int64                   `json:"rate_limit,omitempty"`
	RateLimitDuration  int64                   `json:"rate_limit_duration,omitempty"`
	AdvancedSignatures *bool                   `json:"advanced_signatures,omitempty"`
	ContentType        *string                 `json:"content_type,omitempty"`
	Authentication     *EndpointAuthentication `json:"authentication,omitempty"`
	MtlsClientCert     *MtlsClientCert         `json:"mtls_client_cert,omitempty"`
}

type ExpireSecretRequest struct {
	Secret     string `json:"secret,omitempty"`
	Expiration int64  `json:"expiration"`
}

// ----------------------------------------------------------------- sources

type Source struct {
	UID               string          `json:"uid"`
	ProjectID         string          `json:"project_id"`
	MaskID            string          `json:"mask_id"`
	Name              string          `json:"name"`
	Type              string          `json:"type"`
	Provider          string          `json:"provider"`
	URL               string          `json:"url"`
	IsDisabled        bool            `json:"is_disabled"`
	Verifier          *Verifier       `json:"verifier,omitempty"`
	PubSub            *PubSubConfig   `json:"pub_sub,omitempty"`
	CustomResponse    *CustomResponse `json:"custom_response,omitempty"`
	IdempotencyKeys   []string        `json:"idempotency_keys"`
	ForwardHeaders    []string        `json:"forward_headers"`
	EventTypeLocation string          `json:"event_type_location"`
	BodyFunction      *string         `json:"body_function,omitempty"`
	HeaderFunction    *string         `json:"header_function,omitempty"`
	CreatedAt         string          `json:"created_at"`
	UpdatedAt         string          `json:"updated_at"`
}

type Verifier struct {
	Type      string     `json:"type"`
	HMac      *HMac      `json:"hmac,omitempty"`
	BasicAuth *BasicAuth `json:"basic_auth,omitempty"`
	APIKey    *HeaderKey `json:"api_key,omitempty"`
}

type HMac struct {
	Header   string `json:"header"`
	Hash     string `json:"hash"`
	Secret   string `json:"secret"`
	Encoding string `json:"encoding"`
}

type CustomResponse struct {
	Body        string `json:"body"`
	ContentType string `json:"content_type"`
}

type PubSubConfig struct {
	Type    string        `json:"type"`
	Workers int64         `json:"workers"`
	SQS     *SQSPubSub    `json:"sqs,omitempty"`
	Google  *GooglePubSub `json:"google,omitempty"`
	Kafka   *KafkaPubSub  `json:"kafka,omitempty"`
	AMQP    *AMQPPubSub   `json:"amqp,omitempty"`
}

type SQSPubSub struct {
	AccessKeyID   string `json:"access_key_id"`
	SecretKey     string `json:"secret_key"`
	DefaultRegion string `json:"default_region"`
	QueueName     string `json:"queue_name"`
	Endpoint      string `json:"endpoint,omitempty"`
}

type GooglePubSub struct {
	ProjectID      string `json:"project_id"`
	SubscriptionID string `json:"subscription_id"`
	// ServiceAccount is the base64 encoded service-account JSON.
	ServiceAccount string `json:"service_account"`
}

type KafkaPubSub struct {
	Brokers         []string   `json:"brokers"`
	ConsumerGroupID string     `json:"consumer_group_id,omitempty"`
	TopicName       string     `json:"topic_name"`
	Auth            *KafkaAuth `json:"auth,omitempty"`
}

type KafkaAuth struct {
	Type     string `json:"type,omitempty"`
	Hash     string `json:"hash,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	TLS      bool   `json:"tls"`
}

// AMQPPubSub uses the request shape (bindExchange object). Convoy answers
// with a flattened shape (bindedExchange/routingKey) which UnmarshalJSON
// folds back in.
type AMQPPubSub struct {
	Schema             string        `json:"schema"`
	Host               string        `json:"host"`
	Port               string        `json:"port"`
	Queue              string        `json:"queue"`
	Vhost              string        `json:"vhost,omitempty"`
	DeadLetterExchange string        `json:"deadLetterExchange,omitempty"`
	Auth               *AMQPAuth     `json:"auth,omitempty"`
	BindExchange       *AMQPExchange `json:"bindExchange,omitempty"`
}

func (a *AMQPPubSub) UnmarshalJSON(b []byte) error {
	type plain AMQPPubSub
	var aux struct {
		plain
		BindedExchange string `json:"bindedExchange"`
		RoutingKey     string `json:"routingKey"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*a = AMQPPubSub(aux.plain)
	if a.BindExchange == nil && (aux.BindedExchange != "" || aux.RoutingKey != "") {
		a.BindExchange = &AMQPExchange{Exchange: aux.BindedExchange, RoutingKey: aux.RoutingKey}
	}
	return nil
}

type AMQPAuth struct {
	User     string `json:"user"`
	Password string `json:"password"`
}

type AMQPExchange struct {
	Exchange   string `json:"exchange"`
	RoutingKey string `json:"routingKey"`
}

type SourceRequest struct {
	Name              string          `json:"name"`
	Type              string          `json:"type"`
	Provider          string          `json:"provider,omitempty"`
	IsDisabled        *bool           `json:"is_disabled,omitempty"`
	Verifier          *Verifier       `json:"verifier,omitempty"`
	PubSub            *PubSubConfig   `json:"pub_sub,omitempty"`
	CustomResponse    *CustomResponse `json:"custom_response,omitempty"`
	IdempotencyKeys   []string        `json:"idempotency_keys"`
	ForwardHeaders    []string        `json:"forward_headers,omitempty"`
	EventTypeLocation *string         `json:"event_type_location,omitempty"`
	BodyFunction      *string         `json:"body_function,omitempty"`
	HeaderFunction    *string         `json:"header_function,omitempty"`
}

// ----------------------------------------------------------- subscriptions

type Subscription struct {
	UID             string           `json:"uid"`
	ProjectID       string           `json:"project_id"`
	Name            string           `json:"name"`
	Type            string           `json:"type"`
	DeliveryMode    string           `json:"delivery_mode"`
	Function        *string          `json:"function,omitempty"`
	EndpointID      string           `json:"-"`
	SourceID        string           `json:"-"`
	Endpoint        *Endpoint        `json:"endpoint_metadata,omitempty"`
	Source          *Source          `json:"source_metadata,omitempty"`
	FilterConfig    *FilterConfig    `json:"filter_config,omitempty"`
	AlertConfig     *AlertConfig     `json:"alert_config,omitempty"`
	RetryConfig     *RetryConfig     `json:"retry_config,omitempty"`
	RateLimitConfig *RateLimitConfig `json:"rate_limit_config,omitempty"`
	CreatedAt       string           `json:"created_at"`
	UpdatedAt       string           `json:"updated_at"`
}

// UnmarshalJSON resolves the endpoint/source IDs, which Convoy returns either
// as top level endpoint_id/source_id or only inside the *_metadata objects.
func (s *Subscription) UnmarshalJSON(b []byte) error {
	type plain Subscription
	var aux struct {
		plain
		EndpointID string `json:"endpoint_id"`
		SourceID   string `json:"source_id"`
	}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	*s = Subscription(aux.plain)
	s.EndpointID, s.SourceID = aux.EndpointID, aux.SourceID
	if s.EndpointID == "" && s.Endpoint != nil {
		s.EndpointID = s.Endpoint.UID
	}
	if s.SourceID == "" && s.Source != nil {
		s.SourceID = s.Source.UID
	}
	return nil
}

type FilterConfig struct {
	EventTypes []string      `json:"event_types"`
	Filter     *FilterSchema `json:"filter,omitempty"`
}

// FilterSchema carries arbitrary JSON match documents.
type FilterSchema struct {
	Headers     json.RawMessage `json:"headers,omitempty"`
	Body        json.RawMessage `json:"body,omitempty"`
	Path        json.RawMessage `json:"path,omitempty"`
	Query       json.RawMessage `json:"query,omitempty"`
	IsFlattened bool            `json:"is_flattened,omitempty"`
}

type AlertConfig struct {
	Count     int64  `json:"count"`
	Threshold string `json:"threshold"`
}

// RetryConfig is the response shape; duration is in seconds.
type RetryConfig struct {
	Type       string `json:"type"`
	Duration   int64  `json:"duration"`
	RetryCount int64  `json:"retry_count"`
}

type SubscriptionRequest struct {
	Name            string           `json:"name"`
	EndpointID      string           `json:"endpoint_id,omitempty"`
	SourceID        string           `json:"source_id,omitempty"`
	Function        *string          `json:"function,omitempty"`
	DeliveryMode    string           `json:"delivery_mode,omitempty"`
	FilterConfig    *FilterConfig    `json:"filter_config,omitempty"`
	AlertConfig     *AlertConfig     `json:"alert_config,omitempty"`
	RetryConfig     *RetryConfigReq  `json:"retry_config,omitempty"`
	RateLimitConfig *RateLimitConfig `json:"rate_limit_config,omitempty"`
}

type RetryConfigReq struct {
	Type            string `json:"type,omitempty"`
	IntervalSeconds int64  `json:"interval_seconds,omitempty"`
	RetryCount      int64  `json:"retry_count,omitempty"`
}

// --------------------------------------------------------------- filters

type Filter struct {
	UID            string `json:"uid"`
	SubscriptionID string `json:"subscription_id"`
	EventType      string `json:"event_type"`
	// Raw* hold the documents exactly as submitted; the plain fields hold
	// Convoy's flattened representation.
	RawHeaders json.RawMessage `json:"raw_headers,omitempty"`
	RawBody    json.RawMessage `json:"raw_body,omitempty"`
	RawPath    json.RawMessage `json:"raw_path,omitempty"`
	RawQuery   json.RawMessage `json:"raw_query,omitempty"`
	Headers    json.RawMessage `json:"headers,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
	Path       json.RawMessage `json:"path,omitempty"`
	Query      json.RawMessage `json:"query,omitempty"`
	EnabledAt  string          `json:"enabled_at,omitempty"`
}

type FilterRequest struct {
	EventType string          `json:"event_type"`
	Headers   json.RawMessage `json:"headers,omitempty"`
	Body      json.RawMessage `json:"body,omitempty"`
	Path      json.RawMessage `json:"path,omitempty"`
	Query     json.RawMessage `json:"query,omitempty"`
}

// ------------------------------------------------------------- event types

type EventType struct {
	UID          string          `json:"uid"`
	Name         string          `json:"name"`
	Category     string          `json:"category"`
	Description  string          `json:"description"`
	JSONSchema   json.RawMessage `json:"json_schema,omitempty"`
	DeprecatedAt string          `json:"deprecated_at,omitempty"`
}

type EventTypeRequest struct {
	Name        string          `json:"name,omitempty"`
	Category    string          `json:"category"`
	Description string          `json:"description"`
	JSONSchema  json.RawMessage `json:"json_schema,omitempty"`
}

// ------------------------------------------------------------ portal links

type PortalLink struct {
	UID               string   `json:"uid"`
	ProjectID         string   `json:"project_id"`
	Name              string   `json:"name"`
	OwnerID           string   `json:"owner_id"`
	Endpoints         []string `json:"endpoints"`
	CanManageEndpoint bool     `json:"can_manage_endpoint"`
	AuthType          string   `json:"auth_type"`
	Token             string   `json:"token"`
	AuthKey           string   `json:"auth_key"`
	URL               string   `json:"url"`
	EndpointCount     int64    `json:"endpoint_count"`
	CreatedAt         string   `json:"created_at"`
	UpdatedAt         string   `json:"updated_at"`
}

type PortalLinkRequest struct {
	Name              string   `json:"name"`
	OwnerID           string   `json:"owner_id,omitempty"`
	Endpoints         []string `json:"endpoints,omitempty"`
	CanManageEndpoint bool     `json:"can_manage_endpoint"`
	AuthType          string   `json:"auth_type,omitempty"`
}

// ------------------------------------------------------------------ events

type Event struct {
	UID            string          `json:"uid"`
	ProjectID      string          `json:"project_id"`
	EventType      string          `json:"event_type"`
	Endpoints      []string        `json:"endpoints"`
	IdempotencyKey string          `json:"idempotency_key"`
	Status         string          `json:"status"`
	Data           json.RawMessage `json:"data,omitempty"`
	CreatedAt      string          `json:"created_at"`
}

type SendEventRequest struct {
	EndpointID     string            `json:"endpoint_id,omitempty"`
	OwnerID        string            `json:"owner_id,omitempty"`
	EventType      string            `json:"event_type"`
	Data           json.RawMessage   `json:"data"`
	CustomHeaders  map[string]string `json:"custom_headers,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// ------------------------------------------------------------- pagination

type Pagination struct {
	HasNextPage    bool   `json:"has_next_page"`
	NextPageCursor string `json:"next_page_cursor"`
	PerPage        int64  `json:"per_page"`
}

type paged[T any] struct {
	Content    []T         `json:"content"`
	Pagination *Pagination `json:"pagination"`
}
