package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type ServiceNode struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Owner       string `json:"owner"`
	Status      string `json:"status"` // healthy | degraded | outage
	LatencyMS   int    `json:"latency_ms"`
	Availability float64 `json:"availability"`
}

type TicketEvent struct {
	At      time.Time `json:"at"`
	Actor   string    `json:"actor"`
	Action  string    `json:"action"`
	Comment string    `json:"comment"`
}

type Ticket struct {
	ID          string        `json:"id"`
	Title       string        `json:"title"`
	ServiceID   string        `json:"service_id"`
	Severity    string        `json:"severity"` // P0 | P1 | P2 | P3
	Status      string        `json:"status"`   // open | triaged | investigating | resolved | closed
	Assignee    string        `json:"assignee"`
	Reporter    string        `json:"reporter"`
	Description string        `json:"description"`
	CreatedAt   time.Time     `json:"created_at"`
	UpdatedAt   time.Time     `json:"updated_at"`
	SLADeadline time.Time     `json:"sla_deadline"`
	SLABreached bool          `json:"sla_breached"`
	Timeline    []TicketEvent `json:"timeline"`
}

type AppStore struct {
	mu        sync.RWMutex
	startedAt time.Time
	seq       int
	services  []*ServiceNode
	tickets   []*Ticket
}

func NewAppStore() *AppStore {
	now := time.Now().UTC()
	return &AppStore{
		startedAt: now,
		seq:       104,
		services: []*ServiceNode{
			{ID: "svc-gateway", Name: "API 统一接入网关", Owner: "Atlas", Status: "healthy", LatencyMS: 18, Availability: 99.99},
			{ID: "svc-billing", Name: "实时计费与对账核心", Owner: "Go 后端团队", Status: "degraded", LatencyMS: 245, Availability: 99.42},
			{ID: "svc-router", Name: "AI 模型推理路由", Owner: "Nova", Status: "healthy", LatencyMS: 64, Availability: 99.95},
			{ID: "svc-notify", Name: "飞书消息触达通道", Owner: "Luna", Status: "healthy", LatencyMS: 32, Availability: 99.98},
		},
		tickets: []*Ticket{
			{
				ID:          "INC-101",
				Title:       "实时计费队列出现 P99 延迟抖动（>240ms）",
				ServiceID:   "svc-billing",
				Severity:    "P0",
				Status:      "investigating",
				Assignee:    "Titan（运维部署工程师）",
				Reporter:    "Prometheus-Alert",
				Description: "华东区结算分片连接池饱和，触发 P0 SLA 15 分钟响应倒计时。",
				CreatedAt:   now.Add(-9 * time.Minute),
				UpdatedAt:   now.Add(-3 * time.Minute),
				SLADeadline: now.Add(6 * time.Minute),
				Timeline: []TicketEvent{
					{At: now.Add(-9 * time.Minute), Actor: "AlertRouter", Action: "created", Comment: "自动抓取 P0 告警并建单"},
					{At: now.Add(-6 * time.Minute), Actor: "Titan", Action: "investigating", Comment: "正在检查数据库连接池与慢查询日志"},
				},
			},
			{
				ID:          "INC-102",
				Title:       "官网演示表单重复提交幂等校验优化",
				ServiceID:   "svc-notify",
				Severity:    "P1",
				Status:      "triaged",
				Assignee:    "Luna（运营专员）",
				Reporter:    "Vera（质量专家）",
				Description: "连续快速点击预约演示按钮时需增加请求去重窗口。",
				CreatedAt:   now.Add(-25 * time.Minute),
				UpdatedAt:   now.Add(-15 * time.Minute),
				SLADeadline: now.Add(35 * time.Minute),
				Timeline: []TicketEvent{
					{At: now.Add(-25 * time.Minute), Actor: "Vera", Action: "created", Comment: "QA 回归巡检发现边界用例"},
				},
			},
			{
				ID:          "INC-103",
				Title:       "推理路由灰度节点权重平滑切换演练",
				ServiceID:   "svc-router",
				Severity:    "P2",
				Status:      "resolved",
				Assignee:    "Atlas（首席架构师）",
				Reporter:    "Nova（AI 总经理）",
				Description: "完成金丝雀发布 10% -> 50% -> 100% 流量验证。",
				CreatedAt:   now.Add(-2 * time.Hour),
				UpdatedAt:   now.Add(-20 * time.Minute),
				SLADeadline: now.Add(2 * time.Hour),
				Timeline: []TicketEvent{
					{At: now.Add(-20 * time.Minute), Actor: "Atlas", Action: "resolved", Comment: "全量切流无异常"},
				},
			},
		},
	}
}

func slaWindow(severity string) time.Duration {
	switch severity {
	case "P0":
		return 15 * time.Minute
	case "P1":
		return 60 * time.Minute
	case "P2":
		return 4 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (s *AppStore) snapshotTicketLocked(t *Ticket, now time.Time) Ticket {
	cpy := *t
	cpy.SLABreached = t.Status != "resolved" && t.Status != "closed" && now.After(t.SLADeadline)
	cpy.Timeline = append([]TicketEvent(nil), t.Timeline...)
	return cpy
}

func (s *AppStore) Stats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()
	now := time.Now().UTC()
	openCount, investigatingCount, resolvedCount, p0Active, breached := 0, 0, 0, 0, 0
	for _, t := range s.tickets {
		switch t.Status {
		case "resolved", "closed":
			resolvedCount++
		case "investigating":
			investigatingCount++
			openCount++
		default:
			openCount++
		}
		if t.Severity == "P0" && t.Status != "resolved" && t.Status != "closed" {
			p0Active++
		}
		if t.Status != "resolved" && t.Status != "closed" && now.After(t.SLADeadline) {
			breached++
		}
	}
	healthySvc := 0
	for _, svc := range s.services {
		if svc.Status == "healthy" {
			healthySvc++
		}
	}
	return map[string]any{
		"project_name":        "智能告警工单与SLA监控平台",
		"uptime_seconds":      int(now.Sub(s.startedAt).Seconds()),
		"total_tickets":       len(s.tickets),
		"active_tickets":      openCount,
		"investigating_count": investigatingCount,
		"resolved_count":      resolvedCount,
		"p0_active":           p0Active,
		"sla_breached":        breached,
		"healthy_services":    healthySvc,
		"total_services":      len(s.services),
	}
}

func newMux(store *AppStore, webDir string) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":      "ok",
			"deployed_by": "Titan（运维部署工程师）",
			"built_by":    "智能告警工单与SLA监控平台-Go 后端工程师",
			"service":     "sla-incident-platform",
		})
	})

	mux.HandleFunc("GET /api/stats", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, store.Stats())
	})

	mux.HandleFunc("GET /api/services", func(w http.ResponseWriter, _ *http.Request) {
		store.mu.RLock()
		defer store.mu.RUnlock()
		out := make([]ServiceNode, 0, len(store.services))
		for _, svc := range store.services {
			out = append(out, *svc)
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	})

	mux.HandleFunc("POST /api/services/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Status string `json:"status"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		switch in.Status {
		case "healthy", "degraded", "outage":
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
		id := r.PathValue("id")
		store.mu.Lock()
		defer store.mu.Unlock()
		for _, svc := range store.services {
			if svc.ID == id {
				svc.Status = in.Status
				switch in.Status {
				case "healthy":
					svc.LatencyMS = 22
					svc.Availability = 99.98
				case "degraded":
					svc.LatencyMS = 260
					svc.Availability = 99.35
				case "outage":
					svc.LatencyMS = 5000
					svc.Availability = 95.10
				}
				writeJSON(w, http.StatusOK, svc)
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "service not found"})
	})

	mux.HandleFunc("GET /api/tickets", func(w http.ResponseWriter, r *http.Request) {
		sev := strings.TrimSpace(r.URL.Query().Get("severity"))
		st := strings.TrimSpace(r.URL.Query().Get("status"))
		q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
		now := time.Now().UTC()
		store.mu.RLock()
		defer store.mu.RUnlock()
		out := make([]Ticket, 0, len(store.tickets))
		for i := len(store.tickets) - 1; i >= 0; i-- {
			t := store.tickets[i]
			if sev != "" && t.Severity != sev {
				continue
			}
			if st != "" && t.Status != st {
				continue
			}
			if q != "" && !strings.Contains(strings.ToLower(t.Title+t.ID+t.Assignee+t.ServiceID), q) {
				continue
			}
			out = append(out, store.snapshotTicketLocked(t, now))
		}
		writeJSON(w, http.StatusOK, map[string]any{"items": out})
	})

	mux.HandleFunc("POST /api/tickets", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Title       string `json:"title"`
			ServiceID   string `json:"service_id"`
			Severity    string `json:"severity"`
			Assignee    string `json:"assignee"`
			Reporter    string `json:"reporter"`
			Description string `json:"description"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if strings.TrimSpace(in.Title) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "title is required"})
			return
		}
		if in.Severity == "" {
			in.Severity = "P1"
		}
		if in.ServiceID == "" {
			in.ServiceID = "svc-gateway"
		}
		if in.Assignee == "" {
			in.Assignee = "Titan（运维部署工程师）"
		}
		if in.Reporter == "" {
			in.Reporter = "OnCall-Operator"
		}
		now := time.Now().UTC()
		store.mu.Lock()
		defer store.mu.Unlock()
		store.seq++
		t := &Ticket{
			ID:          fmt.Sprintf("INC-%d", store.seq),
			Title:       strings.TrimSpace(in.Title),
			ServiceID:   in.ServiceID,
			Severity:    in.Severity,
			Status:      "open",
			Assignee:    in.Assignee,
			Reporter:    in.Reporter,
			Description: strings.TrimSpace(in.Description),
			CreatedAt:   now,
			UpdatedAt:   now,
			SLADeadline: now.Add(slaWindow(in.Severity)),
			Timeline: []TicketEvent{
				{At: now, Actor: in.Reporter, Action: "created", Comment: "创建工单并启动 SLA 计时"},
			},
		}
		store.tickets = append(store.tickets, t)
		writeJSON(w, http.StatusCreated, store.snapshotTicketLocked(t, now))
	})

	mux.HandleFunc("POST /api/tickets/{id}/transition", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Status  string `json:"status"`
			Actor   string `json:"actor"`
			Comment string `json:"comment"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		switch in.Status {
		case "open", "triaged", "investigating", "resolved", "closed":
		default:
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
			return
		}
		id := r.PathValue("id")
		now := time.Now().UTC()
		store.mu.Lock()
		defer store.mu.Unlock()
		for _, t := range store.tickets {
			if t.ID == id {
				t.Status = in.Status
				t.UpdatedAt = now
				actor := in.Actor
				if actor == "" {
					actor = t.Assignee
				}
				comment := in.Comment
				if comment == "" {
					comment = "流转至状态 " + in.Status
				}
				t.Timeline = append(t.Timeline, TicketEvent{At: now, Actor: actor, Action: in.Status, Comment: comment})
				writeJSON(w, http.StatusOK, store.snapshotTicketLocked(t, now))
				return
			}
		}
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "ticket not found"})
	})

	if webDir != "" {
		if info, err := os.Stat(filepath.Join(webDir, "index.html")); err == nil && !info.IsDir() {
			fs := http.FileServer(http.Dir(webDir))
			mux.Handle("GET /", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Cache-Control", "no-store")
				fs.ServeHTTP(w, r)
			}))
		}
	}

	return cors(mux)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func main() {
	port := flag.Int("port", 18800, "HTTP listen port")
	webDir := flag.String("web-dir", ".", "Path containing index.html")
	flag.Parse()

	if _, err := os.Stat(filepath.Join(*webDir, "index.html")); err != nil {
		if _, err2 := os.Stat(filepath.Join("../..", "index.html")); err2 == nil {
			*webDir = "../.."
		}
	}

	addr := fmt.Sprintf("127.0.0.1:%d", *port)
	log.Printf("SLA & Incident Platform listening on http://%s (web-dir=%s)", addr, *webDir)
	log.Fatal(http.ListenAndServe(addr, newMux(NewAppStore(), *webDir)))
}
