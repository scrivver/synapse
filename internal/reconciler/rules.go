package reconciler

import (
	"slices"

	"github.com/chunhou/synapse/internal/job"
	"github.com/chunhou/synapse/internal/metadata"
)

// Rule defines a condition-action pair. If Match returns true for a file,
// Action produces a job to bring the file into the desired state.
type Rule struct {
	Name   string
	Match  func(metadata.File) bool
	Action func(metadata.File) job.Job
}

// DefaultRules returns the built-in movement rules.
// hotBucket/coldBucket are the S3 bucket names for each storage tier.
func DefaultRules(hotBucket, coldBucket string) []Rule {
	return []Rule{
		{
			Name: "move-cold-tagged-to-cold",
			Match: func(f metadata.File) bool {
				return slices.Contains(f.Tags, "cold") && !slices.Contains(f.Locations, coldBucket)
			},
			Action: func(f metadata.File) job.Job {
				from := hotBucket
				if len(f.Locations) > 0 {
					from = f.Locations[0]
				}
				return job.NewMoveFileJob(f.ID, from, coldBucket)
			},
		},
		{
			Name: "move-hot-tagged-to-hot",
			Match: func(f metadata.File) bool {
				return slices.Contains(f.Tags, "hot") && !slices.Contains(f.Locations, hotBucket)
			},
			Action: func(f metadata.File) job.Job {
				from := coldBucket
				if len(f.Locations) > 0 {
					from = f.Locations[0]
				}
				return job.NewMoveFileJob(f.ID, from, hotBucket)
			},
		},
	}
}
