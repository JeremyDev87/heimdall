package heimdallassets

import "embed"

// FS contains the public schemas, policy, and onboarding templates used by Heimdall.
//
//go:embed schemas/*.json policies/*.yaml templates/command-artifact/*
var FS embed.FS
