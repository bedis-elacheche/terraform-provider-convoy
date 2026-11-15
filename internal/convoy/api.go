package convoy

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// ---------------------------------------------------------------- projects

func (c *Client) CreateProject(ctx context.Context, orgID string, req CreateProjectRequest) (*CreateProjectResponse, error) {
	var out CreateProjectResponse
	q := url.Values{"orgID": {orgID}}
	err := c.do(ctx, http.MethodPost, "/v1/projects", q, req, &out)
	return &out, err
}

func (c *Client) GetProject(ctx context.Context, id string) (*Project, error) {
	var out Project
	return &out, c.do(ctx, http.MethodGet, projectPath(id), nil, nil, &out)
}

func (c *Client) ListProjects(ctx context.Context, orgID string) ([]Project, error) {
	var out []Project
	q := url.Values{"orgID": {orgID}}
	return out, c.do(ctx, http.MethodGet, "/v1/projects", q, nil, &out)
}

func (c *Client) UpdateProject(ctx context.Context, id string, req UpdateProjectRequest) (*Project, error) {
	var out Project
	if err := c.do(ctx, http.MethodPut, projectPath(id), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) DeleteProject(ctx context.Context, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(id), nil, nil, nil)
}

// --------------------------------------------------------------- endpoints

func (c *Client) CreateEndpoint(ctx context.Context, projectID string, req EndpointRequest) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "endpoints"), nil, req, &out)
}

func (c *Client) GetEndpoint(ctx context.Context, projectID, id string) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodGet, projectPath(projectID, "endpoints", id), nil, nil, &out)
}

func (c *Client) UpdateEndpoint(ctx context.Context, projectID, id string, req EndpointRequest) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "endpoints", id), nil, req, &out)
}

func (c *Client) DeleteEndpoint(ctx context.Context, projectID, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(projectID, "endpoints", id), nil, nil, nil)
}

// ToggleEndpointPause flips an endpoint between "active" and "paused".
func (c *Client) ToggleEndpointPause(ctx context.Context, projectID, id string) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "endpoints", id, "pause"), nil, nil, &out)
}

// ActivateEndpoint re-activates an endpoint that is inactive or paused.
func (c *Client) ActivateEndpoint(ctx context.Context, projectID, id string) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "endpoints", id, "activate"), nil, nil, &out)
}

// ExpireEndpointSecret rotates the endpoint secret.
func (c *Client) ExpireEndpointSecret(ctx context.Context, projectID, id string, req ExpireSecretRequest) (*Endpoint, error) {
	var out Endpoint
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "endpoints", id, "expire_secret"), nil, req, &out)
}

// ListEndpoints lists endpoints, optionally filtered by name query and owner.
func (c *Client) ListEndpoints(ctx context.Context, projectID, nameQuery, ownerID string) ([]Endpoint, error) {
	q := url.Values{}
	if nameQuery != "" {
		q.Set("q", nameQuery)
	}
	if ownerID != "" {
		q.Set("ownerId", ownerID)
	}
	return listAll[Endpoint](ctx, c, projectPath(projectID, "endpoints"), q)
}

// ----------------------------------------------------------------- sources

func (c *Client) CreateSource(ctx context.Context, projectID string, req SourceRequest) (*Source, error) {
	var out Source
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "sources"), nil, req, &out)
}

func (c *Client) GetSource(ctx context.Context, projectID, id string) (*Source, error) {
	var out Source
	return &out, c.do(ctx, http.MethodGet, projectPath(projectID, "sources", id), nil, nil, &out)
}

func (c *Client) UpdateSource(ctx context.Context, projectID, id string, req SourceRequest) (*Source, error) {
	var out Source
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "sources", id), nil, req, &out)
}

func (c *Client) DeleteSource(ctx context.Context, projectID, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(projectID, "sources", id), nil, nil, nil)
}

func (c *Client) ListSources(ctx context.Context, projectID string) ([]Source, error) {
	return listAll[Source](ctx, c, projectPath(projectID, "sources"), url.Values{})
}

// ----------------------------------------------------------- subscriptions

func (c *Client) CreateSubscription(ctx context.Context, projectID string, req SubscriptionRequest) (*Subscription, error) {
	var out Subscription
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "subscriptions"), nil, req, &out)
}

func (c *Client) GetSubscription(ctx context.Context, projectID, id string) (*Subscription, error) {
	var out Subscription
	return &out, c.do(ctx, http.MethodGet, projectPath(projectID, "subscriptions", id), nil, nil, &out)
}

func (c *Client) UpdateSubscription(ctx context.Context, projectID, id string, req SubscriptionRequest) (*Subscription, error) {
	var out Subscription
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "subscriptions", id), nil, req, &out)
}

func (c *Client) DeleteSubscription(ctx context.Context, projectID, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(projectID, "subscriptions", id), nil, nil, nil)
}

// --------------------------------------------------------------- filters

func (c *Client) CreateFilter(ctx context.Context, projectID, subscriptionID string, req FilterRequest) (*Filter, error) {
	var out Filter
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "subscriptions", subscriptionID, "filters"), nil, req, &out)
}

func (c *Client) GetFilter(ctx context.Context, projectID, subscriptionID, id string) (*Filter, error) {
	var out Filter
	return &out, c.do(ctx, http.MethodGet, projectPath(projectID, "subscriptions", subscriptionID, "filters", id), nil, nil, &out)
}

func (c *Client) UpdateFilter(ctx context.Context, projectID, subscriptionID, id string, req FilterRequest) (*Filter, error) {
	var out Filter
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "subscriptions", subscriptionID, "filters", id), nil, req, &out)
}

func (c *Client) DeleteFilter(ctx context.Context, projectID, subscriptionID, id string) error {
	return c.do(ctx, http.MethodDelete, projectPath(projectID, "subscriptions", subscriptionID, "filters", id), nil, nil, nil)
}

// ------------------------------------------------------------- event types

func (c *Client) CreateEventType(ctx context.Context, projectID string, req EventTypeRequest) (*EventType, error) {
	var out EventType
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "event-types"), nil, req, &out)
}

func (c *Client) UpdateEventType(ctx context.Context, projectID, id string, req EventTypeRequest) (*EventType, error) {
	var out EventType
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "event-types", id), nil, req, &out)
}

func (c *Client) DeprecateEventType(ctx context.Context, projectID, id string) (*EventType, error) {
	var out EventType
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "event-types", id, "deprecate"), nil, nil, &out)
}

func (c *Client) ListEventTypes(ctx context.Context, projectID string) ([]EventType, error) {
	var out []EventType
	return out, c.do(ctx, http.MethodGet, projectPath(projectID, "event-types"), nil, nil, &out)
}

// GetEventType finds an event type by ID. Convoy has no single-object read
// endpoint for event types, so this scans the list. It returns a 404 APIError
// when the event type does not exist.
func (c *Client) GetEventType(ctx context.Context, projectID, id string) (*EventType, error) {
	all, err := c.ListEventTypes(ctx, projectID)
	if err != nil {
		return nil, err
	}
	for i := range all {
		if all[i].UID == id {
			return &all[i], nil
		}
	}
	return nil, &APIError{StatusCode: http.StatusNotFound, Method: http.MethodGet, Path: projectPath(projectID, "event-types", id), Message: "event type not found"}
}

// ------------------------------------------------------------ portal links

func (c *Client) CreatePortalLink(ctx context.Context, projectID string, req PortalLinkRequest) (*PortalLink, error) {
	var out PortalLink
	return &out, c.do(ctx, http.MethodPost, projectPath(projectID, "portal-links"), nil, req, &out)
}

func (c *Client) GetPortalLink(ctx context.Context, projectID, id string) (*PortalLink, error) {
	var out PortalLink
	return &out, c.do(ctx, http.MethodGet, projectPath(projectID, "portal-links", id), nil, nil, &out)
}

func (c *Client) UpdatePortalLink(ctx context.Context, projectID, id string, req PortalLinkRequest) (*PortalLink, error) {
	var out PortalLink
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "portal-links", id), nil, req, &out)
}

func (c *Client) RevokePortalLink(ctx context.Context, projectID, id string) error {
	return c.do(ctx, http.MethodPut, projectPath(projectID, "portal-links", id, "revoke"), nil, nil, nil)
}

// RefreshPortalLinkToken issues a new short-lived token for a portal link
// that uses the refresh_token auth type.
func (c *Client) RefreshPortalLinkToken(ctx context.Context, projectID, id string) (string, error) {
	var out string
	return out, c.do(ctx, http.MethodGet, projectPath(projectID, "portal-links", id, "refresh_token"), nil, nil, &out)
}

// ------------------------------------------------------------------ events

// SendEvent sends an event to a single endpoint ("direct"), all endpoints
// sharing an owner ID ("fanout") or every subscribed endpoint ("broadcast").
func (c *Client) SendEvent(ctx context.Context, projectID, mode string, req SendEventRequest) (*Event, error) {
	path := projectPath(projectID, "events")
	switch mode {
	case "fanout":
		path += "/fanout"
	case "broadcast":
		path += "/broadcast"
	}
	var out Event
	return &out, c.do(ctx, http.MethodPost, path, nil, req, &out)
}

func (c *Client) ReplayEvent(ctx context.Context, projectID, id string) (*Event, error) {
	var out Event
	return &out, c.do(ctx, http.MethodPut, projectPath(projectID, "events", id, "replay"), nil, nil, &out)
}

// ------------------------------------------------------------- pagination

const maxPages = 1000

func listAll[T any](ctx context.Context, c *Client, path string, q url.Values) ([]T, error) {
	var all []T
	q.Set("perPage", strconv.Itoa(100))
	q.Set("direction", "next")
	for page := 0; page < maxPages; page++ {
		var out paged[T]
		if err := c.do(ctx, http.MethodGet, path, q, nil, &out); err != nil {
			return nil, err
		}
		all = append(all, out.Content...)
		if out.Pagination == nil || !out.Pagination.HasNextPage || out.Pagination.NextPageCursor == "" {
			break
		}
		q.Set("next_page_cursor", out.Pagination.NextPageCursor)
	}
	return all, nil
}
