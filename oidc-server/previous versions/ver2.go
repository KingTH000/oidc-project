package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/luikyv/go-oidc/pkg/goidc"
	"github.com/luikyv/go-oidc/pkg/provider"
	"golang.org/x/crypto/bcrypt"
)

// InMemoryClientManager implements goidc.ClientManager interface
type InMemoryClientManager struct {
	clients map[string]*goidc.Client
	mu      sync.RWMutex
}

func NewInMemoryClientManager() *InMemoryClientManager {
	return &InMemoryClientManager{
		clients: make(map[string]*goidc.Client),
	}
}

func (m *InMemoryClientManager) Save(ctx context.Context, client *goidc.Client) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clients[client.ID] = client
	return nil
}

func (m *InMemoryClientManager) Client(ctx context.Context, id string) (*goidc.Client, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	client, exists := m.clients[id]
	if !exists {
		log.Printf("Client not found: %s", id)
		return nil, errors.New("client not found")
	}
	log.Printf("Client found: %s (method=%s)", id, client.ClientMeta.TokenAuthnMethod)
	return client, nil
}

func (m *InMemoryClientManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, id)
	return nil
}

// InMemoryAuthnSessionManager implements goidc.AuthnSessionManager
type InMemoryAuthnSessionManager struct {
	sessions map[string]*goidc.AuthnSession
	mu       sync.RWMutex
}

func NewInMemoryAuthnSessionManager() *InMemoryAuthnSessionManager {
	return &InMemoryAuthnSessionManager{
		sessions: make(map[string]*goidc.AuthnSession),
	}
}

func (m *InMemoryAuthnSessionManager) Save(ctx context.Context, session *goidc.AuthnSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

func (m *InMemoryAuthnSessionManager) SessionByCallbackID(ctx context.Context, callbackID string) (*goidc.AuthnSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.CallbackID == callbackID {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryAuthnSessionManager) SessionByAuthCode(ctx context.Context, authCode string) (*goidc.AuthnSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.AuthCode == authCode {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryAuthnSessionManager) SessionByPushedAuthReqID(ctx context.Context, id string) (*goidc.AuthnSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.PushedAuthReqID == id {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryAuthnSessionManager) SessionByCIBAAuthID(ctx context.Context, id string) (*goidc.AuthnSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.CIBAAuthID == id {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryAuthnSessionManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

// InMemoryGrantSessionManager implements goidc.GrantSessionManager
type InMemoryGrantSessionManager struct {
	sessions map[string]*goidc.GrantSession
	mu       sync.RWMutex
}

func NewInMemoryGrantSessionManager() *InMemoryGrantSessionManager {
	return &InMemoryGrantSessionManager{
		sessions: make(map[string]*goidc.GrantSession),
	}
}

func (m *InMemoryGrantSessionManager) Save(ctx context.Context, session *goidc.GrantSession) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[session.ID] = session
	return nil
}

func (m *InMemoryGrantSessionManager) SessionByTokenID(ctx context.Context, tokenID string) (*goidc.GrantSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.TokenID == tokenID {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryGrantSessionManager) SessionByRefreshToken(ctx context.Context, refreshToken string) (*goidc.GrantSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, session := range m.sessions {
		if session.RefreshToken == refreshToken {
			return session, nil
		}
	}
	return nil, errors.New("session not found")
}

func (m *InMemoryGrantSessionManager) Delete(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, id)
	return nil
}

func (m *InMemoryGrantSessionManager) DeleteByAuthCode(ctx context.Context, authCode string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, session := range m.sessions {
		if session.AuthCode == authCode {
			delete(m.sessions, id)
			return nil
		}
	}
	return nil
}

func main() {
	// Generate RSA key for signing
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Create JWKS with private and public information
	jwks := goidc.JSONWebKeySet{
		Keys: []goidc.JSONWebKey{{
			KeyID:     "main-key",
			Key:       key,
			Algorithm: string(goidc.RS256),
		}},
	}

	// JWKS function that returns the server's keys
	jwksFunc := func(_ context.Context) (goidc.JSONWebKeySet, error) {
		return jwks, nil
	}

	// Create custom storage managers
	clientManager := NewInMemoryClientManager()
	authnSessionManager := NewInMemoryAuthnSessionManager()
	grantSessionManager := NewInMemoryGrantSessionManager()

	// Create authentication policy
	policy := goidc.NewPolicy(
		"simple_policy",
		// Setup function - determines if policy should run
		func(_ *http.Request, _ *goidc.Client, _ *goidc.AuthnSession) bool {
			return true
		},
		// Authentication function
		func(w http.ResponseWriter, r *http.Request, session *goidc.AuthnSession) (goidc.AuthnStatus, error) {
			// Get username from form
			username := r.PostFormValue("username")
			if username == "" {
				// Render login page
				w.Header().Set("Content-Type", "text/html")
				w.Write([]byte(`
					<html>
						<body>
							<h2>Login</h2>
							<form method="POST" action="/authorize/` + session.CallbackID + `">
								<input type="text" name="username" placeholder="Username" required />
								<button type="submit">Login</button>
							</form>
						</body>
					</html>
				`))
				return goidc.StatusInProgress, nil
			}

			// Set the user ID
			session.SetUserID(username)
			
			// Grant the requested scopes
			session.GrantScopes(session.Scopes)
			log.Println("Granted scopes:", session.Scopes)
			
			return goidc.StatusSuccess, nil
		},
	)

	// Create the OpenID Provider
	op, err := provider.New(
		goidc.ProfileOpenID,
		"http://localhost:8080",
		jwksFunc,
		// Use custom storage
		provider.WithClientStorage(clientManager),
		provider.WithAuthnSessionStorage(authnSessionManager),
		provider.WithGrantSessionStorage(grantSessionManager),
		// Enable authorization code grant
		provider.WithAuthorizationCodeGrant(),
		// Enable client credentials grant
		provider.WithClientCredentialsGrant(),
		// Enable refresh token grant
		// provider.WithRefreshTokenGrant(
		// 	func(info goidc.GrantInfo, client *goidc.Client) bool {
		// 		return true // Always issue refresh tokens
		// 	},
		// 	3600*24*30, // 30 days
		// ),
		// Add authentication policy
		provider.WithPolicies(policy),
		// Configure scopes
		provider.WithScopes(
			goidc.ScopeOpenID,
			goidc.ScopeProfile,
			goidc.ScopeEmail,
			goidc.ScopeOfflineAccess,
		),
		// Enable Dynamic Client Registration
		provider.WithDCR(
			// Handle function - custom logic during DCR
			func(r *http.Request, id string, meta *goidc.ClientMeta) error {
				log.Printf("Registering new client: %s", id)
				// You can add custom validation or set defaults here
				return nil
			},
			// Validate initial access token function
			func(r *http.Request, initialToken string) error {
				// Validate the token if you require one for registration
				// For this example, we allow open registration
				return nil
			},
		),
		// Enable PKCE
		provider.WithPKCE(goidc.CodeChallengeMethodSHA256, goidc.CodeChallengeMethodPlain),
		// Configure token options
		provider.WithTokenOptions(func(gi goidc.GrantInfo, c *goidc.Client) goidc.TokenOptions {
			return goidc.NewOpaqueTokenOptions(32, 3600)
		}),
		// provider.WithUserInfoSignatureAlgs(goidc.RS256),
	)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	// Hash the client secret
	hashedSecretBytes, err := bcrypt.GenerateFromPassword([]byte("client-secret"), bcrypt.DefaultCost)
	if err != nil {
		log.Fatalf("Failed to hash client secret: %v", err)
	}
	hashedSecret := string(hashedSecretBytes)

	// Register a static client for testing
	staticClient := &goidc.Client{
		ID:             "test-client",
		HashedSecret:   hashedSecret,
		CreatedAtTimestamp: int(time.Now().Unix()),
		ClientMeta: goidc.ClientMeta{
			Name: "Test Client",
			RedirectURIs: []string{
				"http://localhost:3000/auth/callback",
			},
			GrantTypes: []goidc.GrantType{
				goidc.GrantAuthorizationCode,
				goidc.GrantRefreshToken,
			},
			ResponseTypes: []goidc.ResponseType{
				goidc.ResponseTypeCode,
			},
			TokenAuthnMethod: goidc.ClientAuthnSecretPost,
			ScopeIDs:         "openid profile email offline_access",
		},
	}
	
	if err := clientManager.Save(context.Background(), staticClient); err != nil {
		log.Fatalf("Failed to save static client: %v", err)
	}
	

	// Create HTTP server
	mux := http.NewServeMux()
	
	// Register OpenID Provider routes
	mux.Handle("/", op.Handler())


	mux.HandleFunc("/userinfo", func(w http.ResponseWriter, r *http.Request) {
		log.Println("=== /userinfo Request ===")
		log.Println("Method:", r.Method)
		log.Println("Authorization Header:", r.Header.Get("Authorization"))
		
		// Extract token from header
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 {
				token := parts[1]
				log.Println("Token (first 20 chars):", token[:min(20, len(token))])
				log.Println("Token length:", len(token))
				
				// Check if it's a JWT (has 3 parts separated by dots)
				tokenParts := strings.Split(token, ".")
				if len(tokenParts) == 3 {
					log.Println("✅ Token is JWT format")
					
					// Decode header
					headerBytes, err := base64.RawURLEncoding.DecodeString(tokenParts[0])
					if err == nil {
						log.Println("JWT Header:", string(headerBytes))
					}
				} else {
					log.Println("ℹ️ Token is OPAQUE format")
				}
			}
		}
		
		// Call the actual handler
		op.Handler().ServeHTTP(w, r)
		
		log.Println("=== /userinfo Response sent ===")
	})

	
	// Start server
	log.Println("OpenID Provider running on http://localhost:8080")
	log.Println("Well-known endpoint: http://localhost:8080/.well-known/openid-configuration")
	log.Println("DCR endpoint: http://localhost:8080/register")
	
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
