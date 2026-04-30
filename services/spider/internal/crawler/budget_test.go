package crawler

import "testing"

func TestBudgetAllowsAndThenBlocksRunAttempts(t *testing.T) {
	manager := NewCrawlBudgetManager(BudgetConfig{
		Enabled:           true,
		RunMaxAttempts:    2,
		RunMaxSuccesses:   10,
		DomainMaxAttempts: 10,
		BypassMaxRun:      10,
		BypassMaxDomain:   10,
	})

	if decision := manager.AllowFetch("example.com", false); !decision.Allowed {
		t.Fatalf("expected first fetch to be allowed, got %s", decision.Reason)
	}

	if decision := manager.AllowFetch("example.com", false); !decision.Allowed {
		t.Fatalf("expected second fetch to be allowed, got %s", decision.Reason)
	}

	decision := manager.AllowFetch("example.com", false)
	if decision.Allowed {
		t.Fatalf("expected third fetch to be denied by run attempt budget")
	}

	if decision.Reason != "run_attempt_budget_exhausted" {
		t.Fatalf("expected run_attempt_budget_exhausted, got %s", decision.Reason)
	}
}

func TestBudgetBlocksDomainAttempts(t *testing.T) {
	manager := NewCrawlBudgetManager(BudgetConfig{
		Enabled:           true,
		RunMaxAttempts:    10,
		RunMaxSuccesses:   10,
		DomainMaxAttempts: 1,
		BypassMaxRun:      10,
		BypassMaxDomain:   10,
	})

	if decision := manager.AllowFetch("example.com", false); !decision.Allowed {
		t.Fatalf("expected first domain fetch to be allowed, got %s", decision.Reason)
	}

	decision := manager.AllowFetch("example.com", false)
	if decision.Allowed {
		t.Fatalf("expected second domain fetch to be denied")
	}

	if decision.Reason != "domain_attempt_budget_exhausted" {
		t.Fatalf("expected domain_attempt_budget_exhausted, got %s", decision.Reason)
	}
}

func TestBudgetBlocksBypassBudgets(t *testing.T) {
	manager := NewCrawlBudgetManager(BudgetConfig{
		Enabled:           true,
		RunMaxAttempts:    10,
		RunMaxSuccesses:   10,
		DomainMaxAttempts: 10,
		BypassMaxRun:      1,
		BypassMaxDomain:   1,
	})

	if decision := manager.AllowFetch("example.com", true); !decision.Allowed {
		t.Fatalf("expected first bypass fetch to be allowed, got %s", decision.Reason)
	}

	decision := manager.AllowFetch("another.com", true)
	if decision.Allowed {
		t.Fatalf("expected second bypass fetch to be denied")
	}

	if decision.Reason != "bypass_run_budget_exhausted" {
		t.Fatalf("expected bypass_run_budget_exhausted, got %s", decision.Reason)
	}
}

func TestBudgetBlocksRunSuccessBudget(t *testing.T) {
	manager := NewCrawlBudgetManager(BudgetConfig{
		Enabled:           true,
		RunMaxAttempts:    10,
		RunMaxSuccesses:   1,
		DomainMaxAttempts: 10,
		BypassMaxRun:      10,
		BypassMaxDomain:   10,
	})

	if decision := manager.AllowFetch("example.com", false); !decision.Allowed {
		t.Fatalf("expected first fetch to be allowed, got %s", decision.Reason)
	}

	manager.AccountFetch(true)

	decision := manager.AllowFetch("example.com", false)
	if decision.Allowed {
		t.Fatalf("expected fetch to be denied by success budget")
	}

	if decision.Reason != "run_success_budget_exhausted" {
		t.Fatalf("expected run_success_budget_exhausted, got %s", decision.Reason)
	}
}
