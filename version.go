package main

// version is injected at build time via:
//   go build -ldflags "-X main.version=v1.2.3"
// CI sets it to the git tag (GITHUB_REF_NAME).
// Local / dev builds keep the default "dev".
var version = "dev"
