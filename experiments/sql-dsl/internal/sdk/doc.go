// Package sdk stages the service's library promotion candidates: request
// and response conventions proven here in running code before they move
// outward. web.go stages the If-Match precondition parse for go-web-sdk;
// query.go stages the translation from go-web-sdk's parsed query to
// go-database's directives, which lives service-side because go-web-sdk
// cannot import go-database under the dependency line.
package sdk
