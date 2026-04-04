package crawler

import (
	"hash/fnv"
	"math"
	"net/url"
	"strings"

	"github.com/IonelPopJara/search-engine/services/spider/internal/utils"
)

type frontierScoreDetails struct {
	Depth      float64
	Penalty    float64
	TieBreaker float64
	Score      float64
}

func computeFrontierScore(parentScore float64, rawURL string) frontierScoreDetails {
	nextDepth := math.Floor(parentScore) + 1
	penalty := hostAuthorityPenalty(rawURL)
	tieBreaker := stableTieBreaker(rawURL)

	score := nextDepth + utils.FrontierDepthFractionBase + (penalty * utils.FrontierPenaltyScale) + tieBreaker
	maxScore := nextDepth + 0.999999
	if score > maxScore {
		score = maxScore
	}

	return frontierScoreDetails{
		Depth:      nextDepth,
		Penalty:    penalty,
		TieBreaker: tieBreaker,
		Score:      score,
	}
}

func hostAuthorityPenalty(rawURL string) float64 {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		return utils.FrontierDefaultPenalty
	}

	host := strings.ToLower(parsed.Hostname())
	segments := strings.Split(host, ".")
	penalty := 0.50

	if parsed.Scheme == "https" {
		penalty -= 0.08
	}

	if len(segments) > 2 {
		subdomainPenalty := float64(len(segments)-2) * 0.04
		if subdomainPenalty > 0.16 {
			subdomainPenalty = 0.16
		}
		penalty += subdomainPenalty
	}

	tld := ""
	if len(segments) > 0 {
		tld = segments[len(segments)-1]
	}

	switch tld {
	case "gov", "edu":
		penalty -= 0.20
	case "org":
		penalty -= 0.10
	case "com", "net":
		penalty -= 0.03
	case "xyz", "info", "biz", "top":
		penalty += 0.12
	}

	if len(host) > 28 {
		penalty += 0.08
	}

	hyphenCount := strings.Count(host, "-")
	if hyphenCount > 0 {
		hPenalty := float64(hyphenCount) * 0.02
		if hPenalty > 0.10 {
			hPenalty = 0.10
		}
		penalty += hPenalty
	}

	if strings.ContainsAny(host, "0123456789") {
		penalty += 0.06
	}

	trimmedPath := strings.Trim(parsed.Path, "/")
	if trimmedPath != "" {
		pathDepth := len(strings.Split(trimmedPath, "/"))
		if pathDepth > 3 {
			pathPenalty := float64(pathDepth-3) * 0.02
			if pathPenalty > 0.12 {
				pathPenalty = 0.12
			}
			penalty += pathPenalty
		}
	}

	if parsed.RawQuery != "" {
		penalty += 0.06
	}

	if penalty < 0 {
		penalty = 0
	}
	if penalty > 1 {
		penalty = 1
	}

	return penalty
}

func stableTieBreaker(rawURL string) float64 {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(rawURL))

	bucket := hash.Sum32() % utils.FrontierTieBreakerBuckets
	return (float64(bucket) / float64(utils.FrontierTieBreakerBuckets)) * utils.FrontierTieBreakerScale
}
