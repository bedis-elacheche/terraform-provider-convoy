// Package convoytest provides an in-memory fake of the Convoy REST API used
// by the provider's tests. It mimics the response envelope, status codes and
// a few server-side behaviours the provider has to cope with (defaults,
// partially flattened filters, toggling pause, secret rotation...).
package convoytest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"time"
)

const APIKey = "test-api-key"

// Server is a fake Convoy API.
type Server struct {
	*httptest.Server

	mu     sync.Mutex
	seq    int
	store  map[string]map[string]map[string]any // kind -> id -> object
	Events []map[string]any                     // events sent
	Calls  []string                             // "METHOD path" log
}

// NewServer starts a fake server. Call Close when done.
func NewServer() *Server {
	s := &Server{store: map[string]map[string]map[string]any{}}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

// BaseURL is the value for the provider's base_url.
func (s *Server) BaseURL() string { return s.URL + "/api" }

// Get returns a copy of a stored object (kind is e.g. "endpoints").
func (s *Server) Get(kind, id string) map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	return clone(s.store[kind][id])
}

// Put stores an object directly (to simulate out-of-band changes).
func (s *Server) Put(kind, id string, obj map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.store[kind] == nil {
		s.store[kind] = map[string]map[string]any{}
	}
	s.store[kind][id] = obj
}

// Delete removes an object (to simulate out-of-band deletion).
func (s *Server) Delete(kind, id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store[kind], id)
}

// Count returns how many objects of a kind exist.
func (s *Server) Count(kind string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.store[kind])
}

func clone(m map[string]any) map[string]any {
	if m == nil {
		return nil
	}
	b, _ := json.Marshal(m)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

func (s *Server) id(prefix string) string {
	s.seq++
	return fmt.Sprintf("%s%04d", prefix, s.seq)
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

func write(w http.ResponseWriter, code int, msg string, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": code < 300, "message": msg, "data": data})
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls = append(s.Calls, r.Method+" "+r.URL.Path)

	if r.Header.Get("Authorization") != "Bearer "+APIKey {
		write(w, http.StatusUnauthorized, "invalid API key", nil)
		return
	}
	var body map[string]any
	if r.Body != nil && (r.Method == http.MethodPost || r.Method == http.MethodPut) {
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && err.Error() != "EOF" {
			write(w, http.StatusBadRequest, "invalid json: "+err.Error(), nil)
			return
		}
	}
	if body == nil {
		body = map[string]any{}
	}

	p := strings.Split(strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/"), "/"), "/")
	if len(p) == 0 || p[0] != "projects" {
		write(w, http.StatusNotFound, "route not found", nil)
		return
	}
	if len(p) == 1 {
		s.projects(w, r, body)
		return
	}
	projectID := p[1]
	project := s.store["projects"][projectID]
	if project == nil {
		write(w, http.StatusNotFound, "project not found", nil)
		return
	}
	if len(p) == 2 {
		s.project(w, r, projectID, body)
		return
	}
	switch p[2] {
	case "endpoints":
		s.endpoints(w, r, project, p[3:], body)
	case "sources":
		s.sources(w, r, project, p[3:], body)
	case "subscriptions":
		s.subscriptions(w, r, project, p[3:], body)
	case "event-types":
		s.eventTypes(w, r, projectID, p[3:], body)
	case "portal-links":
		s.portalLinks(w, r, projectID, p[3:], body)
	case "events":
		s.events(w, r, projectID, p[3:], body)
	default:
		write(w, http.StatusNotFound, "route not found", nil)
	}
}

// ---------------------------------------------------------------- projects

func defaultProjectConfig() map[string]any {
	return map[string]any{
		"max_payload_read_size":             51200,
		"replay_attacks_prevention_enabled": false,
		"add_event_id_trace_headers":        false,
		"disable_endpoint":                  false,
		"multiple_endpoint_subscriptions":   false,
		"search_policy":                     "720h",
		"ssl":                               map[string]any{"enforce_secure_endpoints": true},
		"ratelimit":                         map[string]any{"count": 1000, "duration": 60},
		"strategy":                          map[string]any{"type": "linear", "duration": 10, "retry_count": 10},
		"signature": map[string]any{"header": "X-Convoy-Signature", "versions": []any{
			map[string]any{"uid": "sigv1", "hash": "SHA256", "encoding": "hex", "created_at": "2024-01-01T00:00:00Z"},
		}},
		"meta_event":      map[string]any{"is_enabled": false, "type": "", "event_type": []any{}, "url": "", "secret": ""},
		"circuit_breaker": map[string]any{"sample_rate": 30, "error_timeout": 30, "failure_threshold": 70, "success_threshold": 5, "observability_window": 5, "minimum_request_count": 10, "consecutive_failure_threshold": 10},
	}
}

func (s *Server) ensure(kind string) map[string]map[string]any {
	if s.store[kind] == nil {
		s.store[kind] = map[string]map[string]any{}
	}
	return s.store[kind]
}

func mergeConfig(dst, patch map[string]any) {
	for k, v := range patch {
		if v == nil {
			continue
		}
		if k == "signature" {
			sig := v.(map[string]any)
			for i, raw := range sig["versions"].([]any) {
				ver := raw.(map[string]any)
				if ver["uid"] == nil || ver["uid"] == "" {
					ver["uid"] = fmt.Sprintf("sigv%d", i+1)
				}
				if ver["created_at"] == nil || ver["created_at"] == "" {
					ver["created_at"] = now()
				}
			}
		}
		dst[k] = v
	}
}

func (s *Server) projects(w http.ResponseWriter, r *http.Request, body map[string]any) {
	orgID := r.URL.Query().Get("orgID")
	if orgID == "" {
		write(w, http.StatusBadRequest, "orgID is required", nil)
		return
	}
	switch r.Method {
	case http.MethodPost:
		if body["name"] == nil || body["name"] == "" {
			write(w, http.StatusBadRequest, "please provide a valid name", nil)
			return
		}
		cfg := defaultProjectConfig()
		if c, ok := body["config"].(map[string]any); ok {
			mergeConfig(cfg, c)
		}
		id := s.id("proj_")
		p := map[string]any{"uid": id, "name": body["name"], "type": body["type"], "logo_url": body["logo_url"],
			"organisation_id": orgID, "config": cfg, "created_at": now(), "updated_at": now()}
		s.ensure("projects")[id] = p
		write(w, http.StatusCreated, "Project created successfully", map[string]any{
			"project": p, "api_key": map[string]any{"uid": s.id("key_"), "key": "CO." + id + ".secretkey", "key_type": "project"},
		})
	case http.MethodGet:
		out := []any{}
		for _, p := range s.store["projects"] {
			if p["organisation_id"] == orgID {
				out = append(out, p)
			}
		}
		write(w, http.StatusOK, "Projects fetched", out)
	}
}

func (s *Server) project(w http.ResponseWriter, r *http.Request, id string, body map[string]any) {
	p := s.store["projects"][id]
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Project fetched", p)
	case http.MethodPut:
		p["name"], p["logo_url"] = body["name"], body["logo_url"]
		if c, ok := body["config"].(map[string]any); ok {
			mergeConfig(p["config"].(map[string]any), c)
		}
		p["updated_at"] = now()
		write(w, http.StatusAccepted, "Project updated", p)
	case http.MethodDelete:
		delete(s.store["projects"], id)
		write(w, http.StatusOK, "Project deleted", nil)
	}
}

// --------------------------------------------------------------- endpoints

func (s *Server) endpoints(w http.ResponseWriter, r *http.Request, project map[string]any, p []string, body map[string]any) {
	store := s.ensure("endpoints")
	projectID := project["uid"].(string)
	if len(p) == 0 {
		switch r.Method {
		case http.MethodPost:
			if body["url"] == nil || body["name"] == nil {
				write(w, http.StatusBadRequest, "please provide a url and name for your endpoint", nil)
				return
			}
			secret, _ := body["secret"].(string)
			if secret == "" {
				secret = "generated-" + s.id("sec_")
			}
			e := map[string]any{
				"uid": s.id("ep_"), "project_id": projectID, "status": "active",
				"http_timeout": 10, "rate_limit": 5000, "rate_limit_duration": 60, "content_type": "application/json",
				"advanced_signatures": project["type"] == "outgoing",
				"secrets":             []any{map[string]any{"uid": s.id("sec_"), "value": secret, "created_at": now()}},
				"created_at":          now(), "updated_at": now(),
			}
			applyEndpoint(e, body)
			store[e["uid"].(string)] = e
			write(w, http.StatusCreated, "Endpoint created successfully", e)
		case http.MethodGet:
			out := []any{}
			for _, e := range sorted(store) {
				if e["project_id"] != projectID {
					continue
				}
				if q := r.URL.Query().Get("q"); q != "" && !strings.Contains(fmt.Sprint(e["name"]), q) {
					continue
				}
				out = append(out, e)
			}
			write(w, http.StatusOK, "Endpoints fetched", map[string]any{"content": out, "pagination": map[string]any{"has_next_page": false}})
		}
		return
	}
	e := store[p[0]]
	if e == nil || e["project_id"] != projectID {
		write(w, http.StatusNotFound, "endpoint not found", nil)
		return
	}
	if len(p) == 2 {
		switch p[1] {
		case "pause":
			switch e["status"] {
			case "active":
				e["status"] = "paused"
			case "paused":
				e["status"] = "active"
			default:
				write(w, http.StatusBadRequest, "current endpoint status does not support pausing", nil)
				return
			}
		case "activate":
			e["status"] = "active"
		case "expire_secret":
			secrets := e["secrets"].([]any)
			last := secrets[len(secrets)-1].(map[string]any)
			last["expires_at"] = now()
			ns, _ := body["secret"].(string)
			if ns == "" {
				ns = "generated-" + s.id("sec_")
			}
			e["secrets"] = append(secrets, map[string]any{"uid": s.id("sec_"), "value": ns, "created_at": now()})
		}
		write(w, http.StatusAccepted, "ok", e)
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Endpoint fetched", e)
	case http.MethodPut:
		applyEndpoint(e, body)
		e["updated_at"] = now()
		write(w, http.StatusAccepted, "Endpoint updated", e)
	case http.MethodDelete:
		delete(store, p[0])
		write(w, http.StatusOK, "Endpoint deleted", nil)
	}
}

func applyEndpoint(e, body map[string]any) {
	for _, k := range []string{"name", "url", "description", "support_email", "slack_webhook_url", "teams_webhook_url", "authentication"} {
		if v, ok := body[k]; ok {
			e[k] = v
		}
	}
	if v, ok := body["owner_id"].(string); ok && v != "" {
		e["owner_id"] = v
	}
	for _, k := range []string{"http_timeout", "rate_limit", "rate_limit_duration"} {
		if v, ok := body[k].(float64); ok && v != 0 {
			e[k] = v
		}
	}
	if v, ok := body["content_type"].(string); ok && v != "" {
		e["content_type"] = v
	}
	if v, ok := body["advanced_signatures"]; ok && v != nil {
		e["advanced_signatures"] = v
	}
	if v, ok := body["mtls_client_cert"].(map[string]any); ok {
		if v["client_cert"] == "" {
			delete(e, "mtls_client_cert")
		} else {
			// Convoy never echoes the private key back.
			e["mtls_client_cert"] = map[string]any{"client_cert": v["client_cert"], "client_key": ""}
		}
	}
	if a, ok := e["authentication"].(map[string]any); ok {
		// Simulate encryption at rest of the OAuth2 client secret.
		if o, ok := a["oauth2"].(map[string]any); ok && o["client_secret"] != nil {
			o["client_secret"] = "encrypted:xxxx"
			if o["grant_type"] == nil || o["grant_type"] == "" {
				o["grant_type"] = "client_credentials"
			}
		}
	}
}

func sorted(m map[string]map[string]any) []map[string]any {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]map[string]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, m[k])
	}
	return out
}

// ----------------------------------------------------------------- sources

func (s *Server) sources(w http.ResponseWriter, r *http.Request, project map[string]any, p []string, body map[string]any) {
	store := s.ensure("sources")
	projectID := project["uid"].(string)
	if len(p) == 0 {
		switch r.Method {
		case http.MethodPost:
			id, mask := s.id("src_"), s.id("mask")
			src := map[string]any{"uid": id, "project_id": projectID, "mask_id": mask, "is_disabled": false,
				"url": "https://ingest.example.com/ingest/" + mask, "idempotency_keys": []any{}, "forward_headers": []any{},
				"custom_response": map[string]any{"body": "", "content_type": ""}, "created_at": now(), "updated_at": now()}
			applySource(src, body)
			src["is_disabled"] = false // not accepted on create
			store[id] = src
			write(w, http.StatusCreated, "Source created", src)
		case http.MethodGet:
			out := []any{}
			for _, x := range sorted(store) {
				if x["project_id"] == projectID {
					out = append(out, x)
				}
			}
			write(w, http.StatusOK, "Sources fetched", map[string]any{"content": out, "pagination": map[string]any{"has_next_page": false}})
		}
		return
	}
	src := store[p[0]]
	if src == nil || src["project_id"] != projectID {
		write(w, http.StatusNotFound, "source not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Source fetched", src)
	case http.MethodPut:
		applySource(src, body)
		src["updated_at"] = now()
		write(w, http.StatusAccepted, "Source updated", src)
	case http.MethodDelete:
		delete(store, p[0])
		write(w, http.StatusOK, "Source deleted", nil)
	}
}

func applySource(src, body map[string]any) {
	for _, k := range []string{"name", "type", "provider", "verifier", "pub_sub", "idempotency_keys", "forward_headers", "event_type_location", "body_function", "header_function", "is_disabled"} {
		if v, ok := body[k]; ok && v != nil {
			src[k] = v
		}
	}
	if v, ok := body["custom_response"]; ok && v != nil {
		src["custom_response"] = v
	}
	if src["type"] == "http" && src["verifier"] == nil {
		src["verifier"] = map[string]any{"type": "noop"}
	}
	if ps, ok := src["pub_sub"].(map[string]any); ok {
		// Convoy returns the AMQP binding flattened.
		if a, ok := ps["amqp"].(map[string]any); ok {
			if b, ok := a["bindExchange"].(map[string]any); ok {
				a["bindedExchange"], a["routingKey"] = b["exchange"], b["routingKey"]
				delete(a, "bindExchange")
			}
		}
	}
}

// ----------------------------------------------------------- subscriptions

// flatten mimics Convoy: nested objects become dotted keys, operator objects
// ({"$gte": 5}) are kept as values.
func flatten(prefix string, in map[string]any, out map[string]any) {
	for k, v := range in {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if m, ok := v.(map[string]any); ok && len(m) > 0 {
			isOp := false
			for mk := range m {
				if strings.HasPrefix(mk, "$") {
					isOp = true
				}
			}
			if !isOp {
				flatten(key, m, out)
				continue
			}
		}
		out[key] = v
	}
}

func flat(v any) any {
	m, ok := v.(map[string]any)
	if !ok {
		return map[string]any{}
	}
	out := map[string]any{}
	flatten("", m, out)
	return out
}

func (s *Server) subscriptions(w http.ResponseWriter, r *http.Request, project map[string]any, p []string, body map[string]any) {
	store := s.ensure("subscriptions")
	projectID := project["uid"].(string)
	if len(p) == 0 && r.Method == http.MethodPost {
		epID, _ := body["endpoint_id"].(string)
		if project["type"] == "outgoing" && s.store["endpoints"][epID] == nil {
			write(w, http.StatusBadRequest, "failed to find endpoint by id", nil)
			return
		}
		if _, ok := body["retry_config"]; ok {
			write(w, http.StatusBadRequest, "unknown field retry_config", nil)
			return
		}
		sub := map[string]any{"uid": s.id("sub_"), "project_id": projectID, "type": "api", "delivery_mode": "at_least_once",
			"filter_config":     map[string]any{"event_types": []any{"*"}, "filter": map[string]any{"headers": map[string]any{}, "body": map[string]any{}, "path": map[string]any{}, "query": map[string]any{}}},
			"alert_config":      map[string]any{"count": 4, "threshold": "1h"},
			"retry_config":      map[string]any{"type": "linear", "duration": 10, "retry_count": 3},
			"rate_limit_config": map[string]any{"count": 1000, "duration": 60},
			"created_at":        now(), "updated_at": now()}
		s.applySubscription(sub, body)
		store[sub["uid"].(string)] = sub
		write(w, http.StatusCreated, "Subscription created", sub)
		return
	}
	if len(p) == 0 {
		write(w, http.StatusMethodNotAllowed, "", nil)
		return
	}
	sub := store[p[0]]
	if sub == nil || sub["project_id"] != projectID {
		write(w, http.StatusNotFound, "subscription not found", nil)
		return
	}
	if len(p) >= 2 && p[1] == "filters" {
		s.filters(w, r, sub, p[2:], body)
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Subscription fetched", sub)
	case http.MethodPut:
		s.applySubscription(sub, body)
		sub["updated_at"] = now()
		write(w, http.StatusAccepted, "Subscription updated", sub)
	case http.MethodDelete:
		delete(store, p[0])
		write(w, http.StatusOK, "Subscription deleted", nil)
	}
}

func (s *Server) applySubscription(sub, body map[string]any) {
	for _, k := range []string{"name", "delivery_mode"} {
		if v, ok := body[k].(string); ok && v != "" {
			sub[k] = v
		}
	}
	if v, ok := body["function"].(string); ok && v != "" {
		sub["function"] = v
	}
	if v, ok := body["endpoint_id"].(string); ok && v != "" {
		// Convoy only returns the endpoint embedded as metadata.
		sub["endpoint_metadata"] = s.store["endpoints"][v]
	}
	if v, ok := body["source_id"].(string); ok && v != "" {
		sub["source_metadata"] = s.store["sources"][v]
	}
	if fc, ok := body["filter_config"].(map[string]any); ok {
		cur := sub["filter_config"].(map[string]any)
		if et, ok := fc["event_types"].([]any); ok && len(et) > 0 {
			cur["event_types"] = et
		}
		if f, ok := fc["filter"].(map[string]any); ok {
			cur["filter"] = map[string]any{"headers": flat(f["headers"]), "body": flat(f["body"]), "path": flat(f["path"]), "query": flat(f["query"]), "is_flattened": true}
		}
	}
	if ac, ok := body["alert_config"].(map[string]any); ok {
		cur := sub["alert_config"].(map[string]any)
		if c, _ := ac["count"].(float64); c > 0 {
			cur["count"] = c
		}
		if t, _ := ac["threshold"].(string); t != "" {
			cur["threshold"] = t
		}
	}
	if rc, ok := body["retry_config"].(map[string]any); ok {
		cur := sub["retry_config"].(map[string]any)
		if t, _ := rc["type"].(string); t != "" {
			cur["type"] = t
		}
		if d, _ := rc["interval_seconds"].(float64); d > 0 {
			cur["duration"] = d
		}
		if c, _ := rc["retry_count"].(float64); c > 0 {
			cur["retry_count"] = c
		}
	}
	if rl, ok := body["rate_limit_config"].(map[string]any); ok {
		sub["rate_limit_config"] = rl
	}
}

// --------------------------------------------------------------- filters

func (s *Server) filters(w http.ResponseWriter, r *http.Request, sub map[string]any, p []string, body map[string]any) {
	store := s.ensure("filters")
	subID := sub["uid"].(string)
	fill := func(f map[string]any) {
		for _, k := range []string{"headers", "body", "path", "query"} {
			if v, ok := body[k]; ok {
				f["raw_"+k] = v
				f[k] = flat(v)
			}
		}
		if et, ok := body["event_type"].(string); ok && et != "" {
			f["event_type"] = et
		}
	}
	if len(p) == 0 && r.Method == http.MethodPost {
		if et, _ := body["event_type"].(string); et == "" {
			write(w, http.StatusBadRequest, "event_type is required", nil)
			return
		}
		f := map[string]any{"uid": s.id("flt_"), "subscription_id": subID, "enabled_at": now()}
		fill(f)
		store[f["uid"].(string)] = f
		write(w, http.StatusCreated, "Filter created", f)
		return
	}
	if len(p) == 0 {
		write(w, http.StatusMethodNotAllowed, "", nil)
		return
	}
	f := store[p[0]]
	if f == nil || f["subscription_id"] != subID {
		write(w, http.StatusNotFound, "filter not found", nil)
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Filter fetched", f)
	case http.MethodPut:
		fill(f)
		write(w, http.StatusAccepted, "Filter updated", f)
	case http.MethodDelete:
		delete(store, p[0])
		write(w, http.StatusOK, "Filter deleted", nil)
	}
}

// ------------------------------------------------------------- event types

func (s *Server) eventTypes(w http.ResponseWriter, r *http.Request, projectID string, p []string, body map[string]any) {
	store := s.ensure("event_types")
	if len(p) == 0 {
		switch r.Method {
		case http.MethodGet:
			out := []any{}
			for _, e := range sorted(store) {
				if e["project_id"] == projectID {
					out = append(out, e)
				}
			}
			write(w, http.StatusOK, "Event types fetched", out)
		case http.MethodPost:
			for _, e := range store {
				if e["project_id"] == projectID && e["name"] == body["name"] {
					write(w, http.StatusBadRequest, "an event type with this name already exists", nil)
					return
				}
			}
			e := map[string]any{"uid": s.id("et_"), "project_id": projectID, "name": body["name"], "category": body["category"],
				"description": body["description"], "json_schema": body["json_schema"]}
			store[e["uid"].(string)] = e
			write(w, http.StatusCreated, "Event type created", e)
		}
		return
	}
	e := store[p[0]]
	if e == nil || e["project_id"] != projectID {
		write(w, http.StatusNotFound, "event type not found", nil)
		return
	}
	if len(p) == 2 && p[1] == "deprecate" {
		e["deprecated_at"] = now()
		write(w, http.StatusOK, "Event type deprecated", e)
		return
	}
	e["category"], e["description"], e["json_schema"] = body["category"], body["description"], body["json_schema"]
	write(w, http.StatusAccepted, "Event type updated", e)
}

// ------------------------------------------------------------ portal links

func (s *Server) portalLinks(w http.ResponseWriter, r *http.Request, projectID string, p []string, body map[string]any) {
	store := s.ensure("portal_links")
	apply := func(pl map[string]any) {
		for _, k := range []string{"name", "owner_id", "can_manage_endpoint"} {
			if v, ok := body[k]; ok {
				pl[k] = v
			}
		}
		eps := []any{}
		if v, ok := body["endpoints"].([]any); ok {
			eps = v
		}
		if owner, _ := pl["owner_id"].(string); owner != "" {
			for _, e := range sorted(s.store["endpoints"]) {
				if e["owner_id"] == owner {
					eps = append(eps, e["uid"])
				}
			}
		}
		pl["endpoints"], pl["endpoint_count"] = eps, len(eps)
	}
	if len(p) == 0 && r.Method == http.MethodPost {
		authType, _ := body["auth_type"].(string)
		if authType == "" {
			authType = "static_token"
		}
		id, tok := s.id("pl_"), s.id("tok_")
		pl := map[string]any{"uid": id, "project_id": projectID, "auth_type": authType, "token": tok,
			"url": "https://portal.example.com/portal?token=" + tok, "created_at": now(), "updated_at": now()}
		apply(pl)
		store[id] = pl
		write(w, http.StatusCreated, "Portal link created", pl)
		return
	}
	if len(p) == 0 {
		write(w, http.StatusMethodNotAllowed, "", nil)
		return
	}
	pl := store[p[0]]
	if pl == nil || pl["project_id"] != projectID {
		write(w, http.StatusNotFound, "portal link not found", nil)
		return
	}
	if len(p) == 2 {
		switch p[1] {
		case "revoke":
			delete(store, p[0])
			write(w, http.StatusOK, "Portal link revoked", nil)
		case "refresh_token":
			write(w, http.StatusOK, "Token refreshed", "refreshed-"+s.id("tok_"))
		}
		return
	}
	switch r.Method {
	case http.MethodGet:
		write(w, http.StatusOK, "Portal link fetched", pl)
	case http.MethodPut:
		apply(pl)
		write(w, http.StatusAccepted, "Portal link updated", pl)
	}
}

// ------------------------------------------------------------------ events

func (s *Server) events(w http.ResponseWriter, r *http.Request, projectID string, p []string, body map[string]any) {
	if len(p) == 2 && p[1] == "replay" {
		for _, e := range s.Events {
			if e["uid"] == p[0] {
				e["replayed"] = true
				write(w, http.StatusOK, "Event replayed", e)
				return
			}
		}
		write(w, http.StatusNotFound, "event not found", nil)
		return
	}
	mode := "direct"
	if len(p) == 1 {
		mode = p[0]
	}
	if mode == "direct" && body["endpoint_id"] == nil {
		write(w, http.StatusBadRequest, "please provide an endpoint ID", nil)
		return
	}
	body["uid"], body["project_id"], body["mode"] = s.id("evt_"), projectID, mode
	s.Events = append(s.Events, body)
	// Like Convoy, the event is enqueued and not returned.
	write(w, http.StatusCreated, "Event queued successfully", nil)
}
