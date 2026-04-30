package crawler

import "sync"

type BudgetConfig struct {
	Enabled           bool
	RunMaxAttempts    int
	RunMaxSuccesses   int
	DomainMaxAttempts int
	BypassMaxRun      int
	BypassMaxDomain   int
}

type BudgetDecision struct {
	Allowed bool
	Reason  string
}

type CrawlBudgetManager struct {
	config BudgetConfig

	mu             sync.Mutex
	runAttempts    int
	runSuccesses   int
	bypassRun      int
	domainAttempts map[string]int
	bypassDomain   map[string]int
}

func NewCrawlBudgetManager(config BudgetConfig) *CrawlBudgetManager {
	manager := &CrawlBudgetManager{
		config:         config,
		domainAttempts: make(map[string]int),
		bypassDomain:   make(map[string]int),
	}

	if manager.config.RunMaxAttempts <= 0 {
		manager.config.RunMaxAttempts = 50000
	}

	if manager.config.RunMaxSuccesses <= 0 {
		manager.config.RunMaxSuccesses = 30000
	}

	if manager.config.DomainMaxAttempts <= 0 {
		manager.config.DomainMaxAttempts = 2000
	}

	if manager.config.BypassMaxRun <= 0 {
		manager.config.BypassMaxRun = 200
	}

	if manager.config.BypassMaxDomain <= 0 {
		manager.config.BypassMaxDomain = 50
	}

	return manager
}

func (m *CrawlBudgetManager) AllowFetch(host string, bypass bool) BudgetDecision {
	if m == nil || !m.config.Enabled {
		return BudgetDecision{Allowed: true, Reason: "budget_disabled"}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runAttempts >= m.config.RunMaxAttempts {
		return BudgetDecision{Allowed: false, Reason: "run_attempt_budget_exhausted"}
	}

	if m.runSuccesses >= m.config.RunMaxSuccesses {
		return BudgetDecision{Allowed: false, Reason: "run_success_budget_exhausted"}
	}

	if host != "" && m.domainAttempts[host] >= m.config.DomainMaxAttempts {
		return BudgetDecision{Allowed: false, Reason: "domain_attempt_budget_exhausted"}
	}

	if bypass {
		if m.bypassRun >= m.config.BypassMaxRun {
			return BudgetDecision{Allowed: false, Reason: "bypass_run_budget_exhausted"}
		}

		if host != "" && m.bypassDomain[host] >= m.config.BypassMaxDomain {
			return BudgetDecision{Allowed: false, Reason: "bypass_domain_budget_exhausted"}
		}
	}

	m.runAttempts++
	if host != "" {
		m.domainAttempts[host]++
	}

	if bypass {
		m.bypassRun++
		if host != "" {
			m.bypassDomain[host]++
		}
	}

	return BudgetDecision{Allowed: true, Reason: "allowed"}
}

func (m *CrawlBudgetManager) AccountFetch(success bool) {
	if m == nil || !m.config.Enabled {
		return
	}

	if !success {
		return
	}

	m.mu.Lock()
	m.runSuccesses++
	m.mu.Unlock()
}

func (m *CrawlBudgetManager) RemainingRunAttempts() int {
	if m == nil || !m.config.Enabled {
		return -1
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	remaining := m.config.RunMaxAttempts - m.runAttempts
	if remaining < 0 {
		return 0
	}

	return remaining
}

func (m *CrawlBudgetManager) RemainingRunSuccesses() int {
	if m == nil || !m.config.Enabled {
		return -1
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	remaining := m.config.RunMaxSuccesses - m.runSuccesses
	if remaining < 0 {
		return 0
	}

	return remaining
}
