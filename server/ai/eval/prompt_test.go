//go:build benchmark

// Prompt eval tests that hit live AI providers. Gated behind the `benchmark`
// build tag to keep them out of CI and default local runs — these tests make
// real API calls (cost $ and wall-clock), so they run only on explicit
// benchmark/eval invocations.
//
// Each Test*Prompt function loads its goldens once, then loops over every
// credentialed (provider, model) pair returned by providersForEval and
// runs the goldens against each pair as a t.Run subtest. Per-pair tallies
// gate against modelThresholds (defaulting to defaultPromptEvalPassRate)
// so a strong provider can hold a tighter bar than a weaker one without
// blocking the whole suite.
//
// See server/ai/README.md for invocation recipes. Provider/flag plumbing
// lives in provider_setup_test.go; cross-cutting helpers live in
// report_test.go.

package eval

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"go.ripls.org/ripls/server/ai"
)

// testDataDir is the path from the eval package's working directory to
// server/test_data/ (where the binary image fixtures live). Image-mode
// prompt tests resolve relative file names here.
const testDataDir = "../../test_data"

// TestExperiencePrompt runs the Experience-generation goldens against
// every credentialed (provider, model) pair.
func TestExperiencePrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadExperienceGoldens("testdata/experience_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestExperiencePrompt", "experience", "experience_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)
					currentTime := firstNonEmpty(c.CurrentTime, goldens.Defaults.CurrentTime)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.ExperienceGeneration, error) {
						return pm.Provider.GenerateExperienceFromText(ctx, c.Prompt, region, currentTime)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateExperienceFromText failed: %v", err)
						return
					}

					t.Logf("prompt: %q → title=%q date=%q time=%q tc=%q loc=%q kw=%v names=%v",
						c.Prompt, gen.Title, gen.Date, gen.Time, gen.TimeConfidence,
						gen.LocationQuery, gen.SearchKeywords, gen.MentionedNames)

					for _, chk := range EvaluateExperience(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestGearPrompt runs the Gear text-generation goldens against every
// credentialed (provider, model) pair.
func TestGearPrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadGearGoldens("testdata/gear_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestGearPrompt", "gear", "gear_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.GearGeneration, error) {
						return pm.Provider.GenerateGearFromText(ctx, c.Prompt, region)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateGearFromText failed: %v", err)
						return
					}

					valueUSD := float32(0)
					if gen.ValueEstimate != nil {
						valueUSD = gen.ValueEstimate.EstimatedValueUSD
					}
					t.Logf("prompt: %q → title=%q cat=%q brand=%q mat=%q weight=%.0fg value=$%.2f",
						c.Prompt, gen.Title, gen.Category, gen.Brand, gen.MaterialCategory,
						gen.WeightGrams, valueUSD)

					for _, chk := range EvaluateGear(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestRequestPrompt runs the Request-generation goldens against every
// credentialed (provider, model) pair.
func TestRequestPrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadRequestGoldens("testdata/request_goldens.json")
	if err != nil {
		t.Fatalf("LoadRequestGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestRequestPrompt", "request", "request_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.RequestGeneration, error) {
						return pm.Provider.GenerateRequestContent(ctx, c.Prompt, region)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateRequestContent failed: %v", err)
						return
					}

					valueUSD := float32(0)
					if gen.ValueEstimate != nil {
						valueUSD = gen.ValueEstimate.EstimatedValueUSD
					}
					t.Logf("prompt: %q → title=%q kw=%v loc=%q value=$%.2f",
						c.Prompt, gen.Title, gen.SearchKeywords, gen.LocationQuery, valueUSD)

					for _, chk := range EvaluateRequest(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestRequestSuggestionsPrompt runs the seed-needs extraction goldens against
// every credentialed (provider, model) pair, gating each on precision and
// recall of the seed_needs list (#2731). Typically run with -provider gemini.
func TestRequestSuggestionsPrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadRequestSuggestionGoldens("testdata/request_suggestion_goldens.json")
	if err != nil {
		t.Fatalf("LoadRequestSuggestionGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestRequestSuggestionsPrompt", "request_suggestion", "request_suggestion_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					res, err := timedCall(t, tally, c.ID, func() (*ai.RequestSuggestionResult, error) {
						return pm.Provider.GenerateRequestSuggestions(ctx, c.Title, c.Description, "")
					})
					if err != nil || res == nil {
						t.Errorf("GenerateRequestSuggestions failed: %v", err)
						return
					}
					prec, rec := seedNeedMetrics(c.Expected.SeedNeeds, res.SeedNeeds)
					t.Logf("prompt: %q → seed_needs=%v (expected %v) precision=%.2f recall=%.2f",
						c.Title, res.SeedNeeds, c.Expected.SeedNeeds, prec, rec)

					for _, chk := range EvaluateRequestSuggestions(c, goldens.Defaults, res) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestCommunityPrompt runs the Community-generation goldens against every
// credentialed (provider, model) pair.
func TestCommunityPrompt(t *testing.T) {
	requireMode(t, "text")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadCommunityGoldens("testdata/community_goldens.json")
	if err != nil {
		t.Fatalf("LoadCommunityGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestCommunityPrompt", "community", "community_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.CommunityGeneration, error) {
						return pm.Provider.GenerateCommunityContent(ctx, c.Prompt, region)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateCommunityContent failed: %v", err)
						return
					}

					t.Logf("prompt: %q → kw=%v", c.Prompt, gen.SearchKeywords)

					for _, chk := range EvaluateCommunity(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestGearImagePrompt runs the image-based DetectGearInImage goldens
// against every credentialed (provider, model) pair.
func TestGearImagePrompt(t *testing.T) {
	requireMode(t, "image")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadGearDetectionGoldens("testdata/gear_image_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearDetectionGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestGearImagePrompt", "gear_image", "gear_image_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					data, err := os.ReadFile(filepath.Join(testDataDir, c.ImageFile))
					if err != nil {
						t.Fatalf("read %s: %v", c.ImageFile, err)
					}

					det, err := timedCall(t, tally, c.ID, func() (*ai.GearDetection, error) {
						return pm.Provider.DetectGearInImage(ctx, &ai.DetectionImage{
							ImageData: data,
							MimeType:  c.MimeType,
						})
					})
					if err != nil || det == nil {
						t.Errorf("DetectGearInImage failed: %v", err)
						return
					}

					valStr := "n/a"
					if det.ValueEstimate != nil {
						valStr = formatValueUSD(det.ValueEstimate.EstimatedValueUSD)
					}
					t.Logf("image: %s → title=%q brand=%q model=%q material=%s weight=%.0fg value=%s desc_len=%d desc=%q",
						c.ImageFile, det.Title, det.Brand, det.Model, det.MaterialCategory, det.WeightGrams, valStr, len(det.Description), det.Description)

					for _, chk := range EvaluateGearDetection(c, goldens.Defaults, det) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestGearWebpagePrompt runs the webpage-based GenerateGearFromWebpage
// goldens against every credentialed (provider, model) pair.
func TestGearWebpagePrompt(t *testing.T) {
	requireMode(t, "webpage")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadGearWebpageGoldens("testdata/gear_webpage_goldens.json")
	if err != nil {
		t.Fatalf("LoadGearWebpageGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestGearWebpagePrompt", "gear_webpage", "gear_webpage_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.GearGeneration, error) {
						return pm.Provider.GenerateGearFromWebpage(ctx, c.PageTitle, c.PageDescription, c.PageBody, region)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateGearFromWebpage failed: %v", err)
						return
					}

					valStr := "n/a"
					if gen.ValueEstimate != nil {
						valStr = formatValueUSD(gen.ValueEstimate.EstimatedValueUSD)
					}
					t.Logf("page: %q → title=%q brand=%q material=%s weight=%.0fg value=%s",
						c.PageTitle, gen.Title, gen.Brand, gen.MaterialCategory, gen.WeightGrams, valStr)

					for _, chk := range EvaluateGearWebpage(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestExperienceImagePrompt runs the image-based GenerateExperienceFromImage
// goldens against every credentialed (provider, model) pair.
func TestExperienceImagePrompt(t *testing.T) {
	requireMode(t, "image")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadExperienceImageGoldens("testdata/experience_image_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceImageGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestExperienceImagePrompt", "experience_image", "experience_image_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)
					currentTime := firstNonEmpty(c.CurrentTime, goldens.Defaults.CurrentTime)

					data, err := os.ReadFile(filepath.Join(testDataDir, c.ImageFile))
					if err != nil {
						t.Fatalf("read %s: %v", c.ImageFile, err)
					}

					gen, err := timedCall(t, tally, c.ID, func() (*ai.ExperienceGeneration, error) {
						return pm.Provider.GenerateExperienceFromImage(ctx, &ai.DetectionImage{
							ImageData: data,
							MimeType:  c.MimeType,
						}, region, c.Notes, currentTime)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateExperienceFromImage failed: %v", err)
						return
					}

					t.Logf("image: %s → title=%q date=%q time=%q tc=%q loc=%q",
						c.ImageFile, gen.Title, gen.Date, gen.Time, gen.TimeConfidence, gen.LocationQuery)

					for _, chk := range EvaluateExperienceImage(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}

// TestExperienceWebpagePrompt runs the webpage-based
// GenerateExperienceFromWebpage goldens against every credentialed
// (provider, model) pair.
func TestExperienceWebpagePrompt(t *testing.T) {
	requireMode(t, "webpage")
	pms := providersForEval(t)
	if len(pms) == 0 {
		t.Skip("no credentialed providers selected")
	}
	goldens, err := LoadExperienceWebpageGoldens("testdata/experience_webpage_goldens.json")
	if err != nil {
		t.Fatalf("LoadExperienceWebpageGoldens: %v", err)
	}
	ctx := context.Background()

	var pairs []PairResult
	defer writeBaselineJSONIfRequested(t, "TestExperienceWebpagePrompt", "experience_webpage", "experience_webpage_goldens.json", goldens.Version, &pairs)

	for _, pm := range pms {
		pm := pm
		t.Run(pm.Key(), func(t *testing.T) {
			tally := NewTally()
			for _, c := range goldens.Cases {
				c := c
				t.Run(c.ID, func(t *testing.T) {
					region := firstNonEmpty(c.Region, goldens.Defaults.Region)
					currentTime := firstNonEmpty(c.CurrentTime, goldens.Defaults.CurrentTime)

					gen, err := timedCall(t, tally, c.ID, func() (*ai.ExperienceGeneration, error) {
						return pm.Provider.GenerateExperienceFromWebpage(ctx, c.PageTitle, c.PageDescription, c.PageBody, region, currentTime)
					})
					if err != nil || gen == nil {
						t.Errorf("GenerateExperienceFromWebpage failed: %v", err)
						return
					}

					t.Logf("page: %q → title=%q date=%q time=%q tc=%q loc=%q",
						c.PageTitle, gen.Title, gen.Date, gen.Time, gen.TimeConfidence, gen.LocationQuery)

					for _, chk := range EvaluateExperienceWebpage(c, goldens.Defaults, gen) {
						tally.Record(chk)
						if !chk.Pass {
							t.Logf("[%s] %s: %s", chk.CaseID, chk.Field, chk.Reason)
						}
					}
				})
			}
			reportAndGate(t, tally, pm.Key(), &pairs)
		})
	}
}
