package deep

import (
	"context"
	"fmt"
	"strings"

	"github.com/esignoretti/s3lytics/internal/s3"
	"github.com/esignoretti/s3lytics/internal/store"
)

func AuditAccess(ctx context.Context, client s3.S3Client, bucket string) (*store.DeepAccessAudit, error) {
	audit := &store.DeepAccessAudit{
		Findings: []store.AccessFinding{},
	}

	pab, err := client.GetPublicAccessBlock(ctx, bucket)
	if err == nil && pab != nil {
		audit.PublicAccessBlocked = true
	} else if err != nil {
		audit.Findings = append(audit.Findings, store.AccessFinding{
			Severity: "WARN",
			Message:  "Public access block not configured",
			Detail:   fmt.Sprintf("GetPublicAccessBlock failed: %v", err),
		})
	}

	policy, err := client.GetBucketPolicy(ctx, bucket)
	if err == nil && policy != "" {
		audit.BucketPolicy = policy
		if strings.Contains(policy, "\"Effect\":\"Allow\"") &&
			strings.Contains(policy, "\"Principal\":\"*\"") {
			audit.Findings = append(audit.Findings, store.AccessFinding{
				Severity: "HIGH",
				Message:  "Bucket policy allows public access",
				Detail:   "Found Effect: Allow with Principal: * in bucket policy",
			})
		}
	}

	grants, err := client.GetBucketAcl(ctx, bucket)
	if err == nil {
		for _, g := range grants {
			if g.Grantee != nil && g.Grantee.Type == "Group" &&
				g.Grantee.URI != nil &&
				strings.Contains(*g.Grantee.URI, "AllUsers") {
				audit.Findings = append(audit.Findings, store.AccessFinding{
					Severity: "HIGH",
					Message:  "Bucket has public ACL grant",
					Detail:   "AllUsers group has permissions on this bucket",
				})
			}
		}
	}

	return audit, nil
}
