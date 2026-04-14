package crawler

import (
	"math"
	"sort"
	"testing"

	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

func TestComputeFrontierScoreStaysWithinDepthBand(t *testing.T) {
	parentScore := 2.487
	details := computeFrontierScore(parentScore, "https://www.stanford.edu/research")

	if details.Depth != 3 {
		t.Fatalf("expected next depth to be 3, got %.3f", details.Depth)
	}

	minExpected := details.Depth + utils.FrontierDepthFractionBase
	maxExpected := details.Depth + 1.0

	if details.Score < minExpected {
		t.Fatalf("score %.4f should be >= %.4f", details.Score, minExpected)
	}

	if details.Score >= maxExpected {
		t.Fatalf("score %.4f should be < %.4f", details.Score, maxExpected)
	}
}

func TestComputeFrontierScorePrefersHigherAuthorityDomains(t *testing.T) {
	parentScore := 0.0
	highAuthority := computeFrontierScore(parentScore, "https://www.mit.edu/about")
	lowAuthority := computeFrontierScore(parentScore, "http://cheap-seo-2025-news-99.biz/free/links?utm_source=spam")

	if !(highAuthority.Penalty < lowAuthority.Penalty) {
		t.Fatalf("expected higher authority URL penalty %.3f to be lower than %.3f", highAuthority.Penalty, lowAuthority.Penalty)
	}

	if !(highAuthority.Score < lowAuthority.Score) {
		t.Fatalf("expected higher authority URL score %.4f to be lower than %.4f", highAuthority.Score, lowAuthority.Score)
	}
}

func TestComputeFrontierScoreDeterministicOrdering(t *testing.T) {
	parentScore := 1.3
	urls := []string{
		"https://www.nasa.gov/mission",
		"https://sub-domain-with-many-parts.example.xyz/deals/2026/03/extra/path?track=1",
		"https://www.cmu.edu/academics",
		"http://example.com/a/b/c/d/e",
	}

	first := scoredURLs(parentScore, urls)
	second := scoredURLs(parentScore, urls)

	for i := range first {
		if first[i].url != second[i].url {
			t.Fatalf("ordering changed at %d: %s != %s", i, first[i].url, second[i].url)
		}

		if math.Abs(first[i].score-second[i].score) > 1e-9 {
			t.Fatalf("score changed for %s: %.10f vs %.10f", first[i].url, first[i].score, second[i].score)
		}
	}
}

type scoredURL struct {
	url   string
	score float64
}

func scoredURLs(parentScore float64, urls []string) []scoredURL {
	result := make([]scoredURL, 0, len(urls))
	for _, currentURL := range urls {
		details := computeFrontierScore(parentScore, currentURL)
		result = append(result, scoredURL{url: currentURL, score: details.Score})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].score == result[j].score {
			return result[i].url < result[j].url
		}

		return result[i].score < result[j].score
	})

	return result
}
