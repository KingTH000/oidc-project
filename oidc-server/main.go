package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"database/sql"
	"embed"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"html/template"
	// "io"
	"log"
	"net/http"
	// "net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/luikyv/go-oidc/pkg/goidc"
	"github.com/luikyv/go-oidc/pkg/provider"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/crypto/bcrypt"

)

//go:embed templates/*.html
var templateFS embed.FS

//go:embed static/css/*.css
var staticFS embed.FS

// User represents a user from the database
type User struct {
	ID        int
	Username  string
	Email     string
	Password  string
	CreatedAt time.Time
}

type Post struct {
	ID        int       `json:"id"`
	UserID    int       `json:"-"` // Exclude from JSON
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type scopeView struct {
	ID          string
	Description string
}
// UserRepository handles user database operations
type UserRepository struct {
	db *sql.DB
}

type PostRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

func NewPostRepository(db *sql.DB) *PostRepository {
	return &PostRepository{db: db}
}

func (r *PostRepository) FindByUserID(ctx context.Context, userID int) ([]Post, error) {
	rows, err := r.db.QueryContext(ctx, "SELECT id, user_id, title, content, created_at FROM posts WHERE user_id = ?", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var posts []Post
	for rows.Next() {
		var p Post
		if err := rows.Scan(&p.ID, &p.UserID, &p.Title, &p.Content, &p.CreatedAt); err != nil {
			return nil, err
		}
		posts = append(posts, p)
	}
	return posts, nil
}

func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*User, error) {
	user := &User{}
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, username, email, password, created_at FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return user, nil
}

func (r *UserRepository) FindByEmail(ctx context.Context, email string) (*User, error) {
	user := &User{}
	err := r.db.QueryRowContext(
		ctx,
		"SELECT id, username, email, password, created_at FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Username, &user.Email, &user.Password, &user.CreatedAt)

	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, fmt.Errorf("database error: %w", err)
	}

	return user, nil
}

func (r *UserRepository) VerifyPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// Template manager
type TemplateManager struct {
	templates *template.Template
}

func NewTemplateManager() (*TemplateManager, error) {
	tmpl, err := template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	return &TemplateManager{templates: tmpl}, nil
}

func (tm *TemplateManager) RenderLogin(w http.ResponseWriter, callbackID string) error {
	data := map[string]interface{}{
		"CallbackID": callbackID,
	}
	return tm.templates.ExecuteTemplate(w, "login.html", data)
}

func (tm *TemplateManager) RenderError(w http.ResponseWriter, message string) error {
	data := map[string]interface{}{
		"Message": message,
	}
	return tm.templates.ExecuteTemplate(w, "error.html", data)
}

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

// MySQLClientManager implements the goidc.ClientManager interface using a SQL database.
type MySQLClientManager struct {
    db *sql.DB
}

func NewMySQLClientManager(db *sql.DB) *MySQLClientManager {
    return &MySQLClientManager{db: db}
}

// Client fetches a client's configuration from the database by its ID.
func (m *MySQLClientManager) Client(ctx context.Context, id string) (*goidc.Client, error) {
    var redirectURIs, grantTypes, responseTypes, scopeIDs string
    client := &goidc.Client{
        ClientMeta: goidc.ClientMeta{},
    }

    err := m.db.QueryRowContext(ctx,
        `SELECT id, hashed_secret, name, redirect_uris, grant_types, response_types, scope_ids, token_authn_method, created_at_timestamp 
         FROM client_apps WHERE id = ?`, id,
    ).Scan(
        &client.ID, &client.HashedSecret, &client.ClientMeta.Name, &redirectURIs, &grantTypes,
        &responseTypes, &scopeIDs, &client.ClientMeta.TokenAuthnMethod, &client.CreatedAtTimestamp,
    )
    if err == sql.ErrNoRows {
        return nil, errors.New("client not found")
    }
    if err != nil {
        return nil, fmt.Errorf("database error fetching client: %w", err)
    }

    // Deserialize string fields back into slices
    client.ClientMeta.RedirectURIs = strings.Split(redirectURIs, ",")
    for _, gt := range strings.Split(grantTypes, ",") {
        client.ClientMeta.GrantTypes = append(client.ClientMeta.GrantTypes, goidc.GrantType(gt))
    }
    for _, rt := range strings.Split(responseTypes, ",") {
        client.ClientMeta.ResponseTypes = append(client.ClientMeta.ResponseTypes, goidc.ResponseType(rt))
    }
    client.ClientMeta.ScopeIDs = scopeIDs

    return client, nil
}

// Save stores a new client or updates an existing one in the database.
func (m *MySQLClientManager) Save(ctx context.Context, client *goidc.Client) error {
    // Serialize slice fields into comma-separated strings
    redirectURIs := strings.Join(client.ClientMeta.RedirectURIs, ",")
    var grantTypes []string
    for _, gt := range client.ClientMeta.GrantTypes {
        grantTypes = append(grantTypes, string(gt))
    }
    grantTypesStr := strings.Join(grantTypes, ",")
    var responseTypes []string
    for _, rt := range client.ClientMeta.ResponseTypes {
        responseTypes = append(responseTypes, string(rt))
    }
    responseTypesStr := strings.Join(responseTypes, ",")

    _, err := m.db.ExecContext(ctx,
        `INSERT INTO client_apps (id, hashed_secret, name, redirect_uris, grant_types, response_types, scope_ids, token_authn_method, created_at_timestamp) 
         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
         ON DUPLICATE KEY UPDATE 
         hashed_secret = VALUES(hashed_secret), name = VALUES(name), redirect_uris = VALUES(redirect_uris), 
         grant_types = VALUES(grant_types), response_types = VALUES(response_types), scope_ids = VALUES(scope_ids), 
         token_authn_method = VALUES(token_authn_method)`,
        client.ID, client.HashedSecret, client.ClientMeta.Name, redirectURIs, grantTypesStr,
        responseTypesStr, client.ClientMeta.ScopeIDs, client.ClientMeta.TokenAuthnMethod, client.CreatedAtTimestamp,
    )

    if err != nil {
        return fmt.Errorf("database error saving client: %w", err)
    }
    return nil
}

// Delete removes a client from the database.
func (m *MySQLClientManager) Delete(ctx context.Context, id string) error {
    _, err := m.db.ExecContext(ctx, "DELETE FROM client_apps WHERE id = ?", id)
    return err
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

// loadOrGenerateKey loads an existing RSA key from file or generates a new one
func loadOrGenerateKey(filename string) (*rsa.PrivateKey, error) {
	// Try to load existing key
	if _, err := os.Stat(filename); err == nil {
		log.Printf("Loading existing RSA key from %s", filename)
		keyData, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}

		block, _ := pem.Decode(keyData)
		if block == nil {
			return nil, errors.New("failed to decode PEM block")
		}

		key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to parse private key: %w", err)
		}

		log.Println("✅ RSA key loaded successfully")
		return key, nil
	}

	// Generate new key if file doesn't exist
	log.Printf("Generating new RSA key and saving to %s", filename)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key: %w", err)
	}

	// Save key to file
	keyBytes := x509.MarshalPKCS1PrivateKey(key)
	pemBlock := &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: keyBytes,
	}

	keyFile, err := os.Create(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to create key file: %w", err)
	}
	defer keyFile.Close()

	if err := pem.Encode(keyFile, pemBlock); err != nil {
		return nil, fmt.Errorf("failed to encode key: %w", err)
	}

	// Set restrictive permissions (only owner can read/write)
	if err := os.Chmod(filename, 0600); err != nil {
		log.Printf("⚠️  Warning: failed to set key file permissions: %v", err)
	}

	log.Println("✅ RSA key generated and saved successfully")
	return key, nil
}

// RenderConsent renders the consent page
func (tm *TemplateManager) RenderConsent(w http.ResponseWriter, data map[string]interface{}) error {
	return tm.templates.ExecuteTemplate(w, "consent.html", data)
}

func main() {
	tmplManager, err := NewTemplateManager()
	if err != nil {
		log.Fatalf("Failed to initialize template manager: %v", err)
	}

	// Connect to MySQL database
	dsn := "root:@tcp(localhost:3306)/blog?parseTime=true"
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}
	log.Println("✅ Connected to MySQL database")

	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(5 * time.Minute)

	userRepo := NewUserRepository(db)
	postRepo := NewPostRepository(db)

	// Generate RSA key for signing
	key, err := loadOrGenerateKey("signing_key.pem")
	if err != nil {
		log.Fatalf("Failed to generate RSA key: %v", err)
	}

	jwks := goidc.JSONWebKeySet{
		Keys: []goidc.JSONWebKey{{
			KeyID:     "main-key",
			Key:       key,
			Algorithm: string(goidc.RS256),
			Use:       "sig",
		}},
	}

	jwksFunc := func(_ context.Context) (goidc.JSONWebKeySet, error) {
		return jwks, nil
	}

	// clientManager := NewInMemoryClientManager()
	clientManager := NewMySQLClientManager(db)
	authnSessionManager := NewInMemoryAuthnSessionManager()
	grantSessionManager := NewInMemoryGrantSessionManager()

	// define scope descriptions
	scopeDescriptions := map[string]string{
		goidc.ScopeOpenID.ID:        "Sign you in to the application.",
		goidc.ScopeProfile.ID:       "Access your basic profile information (e.g., your name).",
		goidc.ScopeEmail.ID:         "Access your email address.",
		goidc.ScopeOfflineAccess.ID: "Access your data even when you are offline.",
		"posts.read":             "Read your blog posts.",
		"contacts.read":          "Read your contacts list.",
	}

	// Create authentication policy with database verification
	policy := goidc.NewPolicy(
		"database_auth_policy",
		func(_ *http.Request, _ *goidc.Client, _ *goidc.AuthnSession) bool {
			return true
		},
		func(w http.ResponseWriter, r *http.Request, session *goidc.AuthnSession) (goidc.AuthnStatus, error) {

			if session.Subject == "" {
                username := r.PostFormValue("username")
                password := r.PostFormValue("password")

				if username == "" || password == "" {
					// Render login page
					w.Header().Set("Content-Type", "text/html")
					if err := tmplManager.RenderLogin(w, session.CallbackID); err != nil {
						log.Printf("Failed to render login: %v", err)
						http.Error(w, "Internal server error", http.StatusInternalServerError)
					}
					return goidc.StatusInProgress, nil
				}

				// Look up user in database
				ctx := r.Context()
				user, err := userRepo.FindByUsername(ctx, username)
				if err != nil {
					user, err = userRepo.FindByEmail(ctx, username)
					if err != nil {
						log.Printf("❌ Login failed: user not found for '%s'", username)
						w.Header().Set("Content-Type", "text/html")
						if err := tmplManager.RenderError(w, "Invalid username or password"); err != nil {
							log.Printf("Failed to render error: %v", err)
						}
						return goidc.StatusInProgress, nil
					}
				}

				log.Printf("✅ User found: %s (ID: %d, Email: %s)", user.Username, user.ID, user.Email)

				// Verify password
				err = userRepo.VerifyPassword(user.Password, password)
				if err != nil {
					log.Printf("❌ Login failed: invalid password for user '%s'", username)
					w.Header().Set("Content-Type", "text/html")
					if err := tmplManager.RenderError(w, "Invalid username or password"); err != nil {
						log.Printf("Failed to render error: %v", err)
					}
					return goidc.StatusInProgress, nil
				}

				log.Printf("✅ Password verified for user '%s'", user.Username)

				session.SetUserID(fmt.Sprintf("%d", user.ID))

				// Populate claims (do this only once after login)
				session.AdditionalIDTokenClaims = map[string]any{"email": user.Email, "username": user.Username}
				
				session.AdditionalUserInfoClaims= map[string]any{"email": user.Email, "username": user.Username}

				session.AdditionalTokenClaims = map[string]any{"aud": session.ClientID}
				
			}
			
			// Check if the consent form has been submitted
			if r.PostFormValue("consent_form") == "" {
				// Consent form not submitted, so we need to show it.
				log.Println("Displaying consent screen...")

				// Safely fetch the client details using the clientManager
                client, err := clientManager.Client(r.Context(), session.ClientID)
                if err != nil {
                    http.Error(w, "Client not found", http.StatusInternalServerError)
                    return goidc.StatusFailure, err
                }
				

				// Create a slice of our custom scopeView struct for the template.
				var scopesForTemplate []scopeView
				log.Println("Requested scopes:", session.Scopes)

				 // Split the single string of scopes into a slice of strings.
    			requestedScopes := strings.Fields(session.Scopes)

				for _, scope := range requestedScopes {
					
					// Look up the description from our map.
					description, ok := scopeDescriptions[string(scope)]
					if !ok {
						// Provide a fallback for any scopes we didn't define a description for.
						description = "Perform an undefined action."
					}
					scopesForTemplate = append(scopesForTemplate, scopeView{
						ID:          string(scope),
						Description: description,
					})
				}
				// Prepare data for the template
				data := map[string]any{
					"CallbackID": session.CallbackID,
					"ClientName": client.ClientMeta.Name,
					"Scopes":     scopesForTemplate,
				}
				if err := tmplManager.RenderConsent(w, data); err != nil {
					// ... error handling
					http.Error(w, "Could not render consent page:(", http.StatusInternalServerError)
				}
				// Pause the flow to wait for user's consent
				return goidc.StatusInProgress, nil
			}

			// Consent form was submitted, process the decision
        	if r.PostFormValue("consent") == "allow" {

				log.Println("✅ User granted consent.")

				session.GrantScopes(session.Scopes)
				log.Println("✅ Granted scopes:", session.Scopes)

				return goidc.StatusSuccess, nil
			}

			// User denied consent
			log.Println("❌ User denied consent.")
			return goidc.StatusFailure, errors.New("user denied access")
		
		},
	)

	op, err := provider.New(
		goidc.ProfileOpenID,
		"http://localhost:8080",
		jwksFunc,
		provider.WithClientStorage(clientManager),
		provider.WithAuthnSessionStorage(authnSessionManager),
		provider.WithGrantSessionStorage(grantSessionManager),
		provider.WithAuthorizationCodeGrant(),
		provider.WithClientCredentialsGrant(),
		provider.WithRefreshTokenGrant(
			func(c *goidc.Client, gi goidc.GrantInfo) bool { return true },
			// New refresh tokens expire in 30 days
			3600*24*30,
		),
		provider.WithPolicies(policy),
		provider.WithScopes(
			goidc.ScopeOpenID,
			goidc.ScopeProfile,
			goidc.ScopeEmail,
			goidc.ScopeOfflineAccess,
			goidc.NewScope(
				"posts.read",
			),
			goidc.Scope{
				ID: "posts.read",},
		),

		provider.WithDCR(
			func(r *http.Request, id string, meta *goidc.ClientMeta) error {
				log.Printf("Registering new client: %s", id)
				return nil
			},
			func(r *http.Request, initialToken string) error {
				return nil
			},
		),
		provider.WithPKCE(goidc.CodeChallengeMethodSHA256, goidc.CodeChallengeMethodPlain),
		provider.WithTokenOptions(func(gi goidc.GrantInfo, c *goidc.Client) goidc.TokenOptions {
			return goidc.NewJWTTokenOptions(goidc.RS256, 3600)
		}),

		// provider.WithTokenIntrospection(
		// 	func (*goidc.Client, goidc.TokenInfo) bool {
		// 		 return true },
		// 	goidc.ClientAuthnSecretPost,
		// ),
		//provider.WithTokenIntrospectionEndpoint("http://localhost:8080/introspect"),
	)
	if err != nil {
		log.Fatalf("Failed to create provider: %v", err)
	}
	

	mux := http.NewServeMux()

	// Serve static files (CSS)
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))

	// Register OpenID Provider routes
	mux.Handle("/", op.Handler())

	go func() {
		if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatalf("Server failed: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond) // Give server a moment to start

	// Initialize a verifier for our JWT access tokens.
	// It uses the OIDC discovery endpoint to find the JWKS URL.
	ctx := context.Background()
	provider, err := oidc.NewProvider(ctx, "http://localhost:8080")
	if err != nil {
		log.Fatalf("Failed to create OIDC provider verifier: %v", err)
	}
	jwtVerifier := provider.Verifier(&oidc.Config{ClientID: "test-client"})
	log.Println("✅ JWT Verifier initialized successfully")

	// When client reach for the api/posts endpoint
	mux.HandleFunc("/api/posts", func(w http.ResponseWriter, r *http.Request) {
		// Get the JWT from the Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Authorization header is missing or invalid", http.StatusUnauthorized)
			return
		}
		jwtString := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the JWT locally using the verifier
		// The verifier checks the signature against the JWKS, the expiration, and the issuer.
		accessToken, err := jwtVerifier.Verify(r.Context(), jwtString)
		if err != nil {
			log.Printf("Token verification failed: %v", err)
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Extract custom claims from the token (including scope)
		var claims struct {
			Scope string `json:"scope"`
		}
		if err := accessToken.Claims(&claims); err != nil {
			http.Error(w, "Failed to extract claims from token", http.StatusInternalServerError)
			return
		}

		// Enforce scope from the the JWT's "scope" claim
		grantedScopes := strings.Fields(claims.Scope)
		hasScope := false
		for _, scope := range grantedScopes {
			if scope == "posts.read" {
				hasScope = true
				break
			}
		}
		if !hasScope {
			http.Error(w, "Insufficient scope", http.StatusForbidden)
			return
		}

		// Get user ID from the JWT's "sub" (subject) claim and proceed
		userID, err := strconv.Atoi(accessToken.Subject)
		if err != nil {
			http.Error(w, "Invalid user ID in token", http.StatusInternalServerError)
			return
		}

		posts, err := postRepo.FindByUserID(r.Context(), userID)
		if err != nil {
			log.Printf("Error fetching posts: %v", err)
			http.Error(w, "Could not fetch posts", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(posts)
	})

	

	log.Println("OpenID Provider running on http://localhost:8080")
	log.Println("Well-known endpoint: http://localhost:8080/.well-known/openid-configuration")
	log.Println("JWKS endpoint: http://localhost:8080/jwks")
	log.Println("Database: blog (users table)")
    
	select{}
    
}
