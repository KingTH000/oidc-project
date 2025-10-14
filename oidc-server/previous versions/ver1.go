package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"html/template"
	"net/http"

	"github.com/luikyv/go-oidc/pkg/goidc"
	"github.com/luikyv/go-oidc/pkg/provider"
	"github.com/luikyv/go-oidc/examples/ui"
)

func test() {
	// 1. Generate RSA signing key
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	jwks := goidc.JSONWebKeySet{
		Keys: []goidc.JSONWebKey{{
			KeyID:     "key_id",
			Key:       key.Public(),
			Algorithm: "RS256",
		}},
	}


clientManager := NewInMemoryClientManager()
// Instantiate the default client manager (in-memory)
// clientStore := provider.WithClientStorage(*clientManager)

// Register a client
err := clientManager.Save(context.Background(), &goidc.Client{
    ID:           "my-client",
    Secret:       "my-secret",
	ClientMeta: goidc.ClientMeta{
			Name: "Test Client",
    		RedirectURIs: []string{"http://localhost:8081/callback"},
	},
    // … any other client metadata you need
})
if err != nil {
    panic(err)
}



	// 3. Simple Authentication Policy
	policy := goidc.NewPolicy(
    "simple_policy",
    func(_ *http.Request, _ *goidc.Client, _ *goidc.AuthnSession) bool {
        return true // always trigger this policy
    },
    func(w http.ResponseWriter, r *http.Request, as *goidc.AuthnSession) (goidc.AuthnStatus, error) {
        username := r.FormValue("username")
        login := r.FormValue("login")

        // if the user has not submitted the login form yet, render login page
        if username == "" && login == "" {
            tmpl := template.Must(template.ParseFS(ui.FS, "login.html"))
            data := map[string]interface{}{
                "BaseURL":  "http://localhost",
                "CallbackID": as.CallbackID,
                "Error":    "",
                "LogoURI":  "",
                "Session":  map[string]string{"client_id": as.ClientID},
            }
            tmpl.Execute(w, data)
            return goidc.StatusInProgress, nil
        }

        // handle user denial
        if login == "false" {
            return goidc.StatusFailure, errors.New("user denied access")
        }

        // authenticate user (you can replace this with a real password check)
        if username == "banned_user" {
            return goidc.StatusFailure, errors.New("the user is banned")
        }

        // authentication success
        as.Subject = username
        as.Claims.IDToken["name"] = goidc.ClaimObjectInfo{Value: username}
        as.Claims.IDToken["email"] = goidc.ClaimObjectInfo{Value: username + "@example.com"}
        as.Claims.UserInfo["name"] = goidc.ClaimObjectInfo{Value: username}
        as.Claims.UserInfo["email"] = goidc.ClaimObjectInfo{Value: username + "@example.com"}

        return goidc.StatusSuccess, nil
    },
)


	// 4. Create the provider
	op, err := provider.New(
		goidc.ProfileOpenID,
		"http://localhost",
		func(_ context.Context) (goidc.JSONWebKeySet, error) {
			return jwks, nil
		},
		provider.WithClientStorage(*new(goidc.ClientManager)),              // register our test client
		provider.WithAuthorizationCodeGrant(),    // enable auth code flow
		provider.WithPolicies(policy),               // attach login policy
		provider.WithScopes(goidc.ScopeOpenID, goidc.ScopeProfile, goidc.ScopeEmail),
	)
	if err != nil {
		panic(err)
	}


	// 5. Run the provider
	fmt.Println("OIDC Provider running at http://localhost")
	if err := op.Run(":80"); err != nil {
		panic(err)
	}
}
