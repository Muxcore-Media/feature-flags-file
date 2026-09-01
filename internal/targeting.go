package internal

import (
	"context"
	"hash/fnv"
	"strings"

	"google.golang.org/grpc/metadata"
)

const callerIDMetadataKey = "x-caller-id"

func callerIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	ids := md.Get(callerIDMetadataKey)
	if len(ids) == 0 {
		return ""
	}
	return strings.TrimSpace(ids[0])
}

func flagEnabledForCaller(rule flagRule, callerID, flag string) bool {
	if len(rule.EnabledFor) > 0 && callerID != "" {
		for _, id := range rule.EnabledFor {
			if id == callerID {
				return true
			}
		}
	}
	if rule.Percent > 0 {
		if callerID == "" {
			return false
		}
		return rolloutBucket(callerID, flag) < rule.Percent
	}
	return rule.Default
}

func rolloutBucket(callerID, flag string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(callerID + ":" + flag))
	return int(h.Sum32() % 100)
}
