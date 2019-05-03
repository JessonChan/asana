// Copyright 2019 asana Author. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package authz provides handlers to enable ACL, RBAC, ABAC authorization support.
// Simple Usage:
//	import(
//		"github.com/goasana/framework"
//		"github.com/goasana/framework/plugins/authz"
//		"github.com/casbin/casbin"
//	)
//
//	func main(){
//		// mediate the access for every request
//		asana.InsertFilter("*", asana.BeforeRouter, authz.NewAuthorizer(casbin.NewEnforcer("authz_model.conf", "authz_policy.csv")))
//		asana.Run()
//	}
//
//
// Advanced Usage:
//
//	func main(){
//		e := casbin.NewEnforcer("authz_model.conf", "")
//		e.AddRoleForUser("alice", "admin")
//		e.AddPolicy(...)
//
//		asana.InsertFilter("*", asana.BeforeRouter, authz.NewAuthorizer(e))
//		asana.Run()
//	}
package authz

import (
	"net/http"

	"github.com/casbin/casbin"
	"github.com/goasana/framework"
	"github.com/goasana/framework/context"
)

// NewAuthorizer returns the authorizer.
// Use a casbin enforcer as input
func NewAuthorizer(e *casbin.Enforcer) asana.FilterFunc {
	return func(ctx *context.Context) {
		a := &BasicAuthorizer{enforcer: e}

		if !a.CheckPermission(ctx.HTTPRequest) {
			a.RequirePermission(ctx.ResponseWriter)
		}
	}
}

// BasicAuthorizer stores the casbin handler
type BasicAuthorizer struct {
	enforcer *casbin.Enforcer
}

// GetUserName gets the user name from the request.
// Currently, only HTTP basic authentication is supported
func (a *BasicAuthorizer) GetUserName(r *http.Request) string {
	username, _, _ := r.BasicAuth()
	return username
}

// CheckPermission checks the user/method/path combination from the request.
// Returns true (permission granted) or false (permission forbidden)
func (a *BasicAuthorizer) CheckPermission(r *http.Request) bool {
	user := a.GetUserName(r)
	method := r.Method
	path := r.URL.Path
	return a.enforcer.Enforce(user, path, method)
}

// RequirePermission returns the 403 Forbidden to the client
func (a *BasicAuthorizer) RequirePermission(w http.ResponseWriter) {
	w.WriteHeader(403)
	_, _ = w.Write([]byte("403 Forbidden\n"))
}
