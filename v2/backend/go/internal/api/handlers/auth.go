// // backend/go/internal/api/middleware/auth.go
// package middleware

// import (
// 	"net/http"
// 	"sage-ai-v2/pkg/logger"
// )

// // ApplyAuth is a middleware that applies authentication to the API endpoints
// func ApplyAuth(next http.Handler) http.Handler {
// 	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
// 		// Set CORS headers for all responses
// 		w.Header().Set("Access-Control-Allow-Origin", "*")
// 		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
// 		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

// 		// Handle preflight requests
// 		if r.Method == "OPTIONS" {
// 			w.WriteHeader(http.StatusOK)
// 			return
// 		}

// 		// Log the request
// 		logger.InfoLogger.Printf("Request: %s %s", r.Method, r.URL.Path)

//			// Continue to the next handler
//			next.ServeHTTP(w, r)
//		})
//	}
//
// backend/go/internal/api/handlers/auth.go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"sage-ai-v2/internal/models"
	"sage-ai-v2/internal/services"
	"sage-ai-v2/pkg/logger"
)

// Define the interface for AuthService
type AuthServiceInterface interface {
    SignIn(ctx context.Context, req models.SignInRequest) (*models.AuthResponse, error)
    SignUp(ctx context.Context, req models.SignUpRequest) (*models.AuthResponse, error)
    OAuthSignIn(ctx context.Context, provider, code, redirectURI string) (*models.AuthResponse, error)
    GetOAuthURL(provider, redirectURI string) (string, error)
    VerifyToken(token string) (string, error)
    GetUserByID(ctx context.Context, id string) (*models.User, error)
}

type AuthHandler struct {
	authService AuthServiceInterface
}

// // Constructor function
// func NewAuthHandler(authService *services.AuthService) *AuthHandler {
// 	return &AuthHandler{authService: authService}
// }

func NewAuthHandler(authService AuthServiceInterface) *AuthHandler {
    return &AuthHandler{authService: authService}
}

// SignInHandler handles user sign-in with email/password
func (h *AuthHandler) SignInHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Credentials", "true")

	// Handle preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check request method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req models.SignInRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.ErrorLogger.Printf("Error parsing sign-in request: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Sign in user
	ctx := r.Context()
	resp, err := h.authService.SignIn(ctx, req)
	if err != nil {
		// Handle specific errors
		switch err {
		case services.ErrUserNotFound:
			http.Error(w, "User not found", http.StatusNotFound)
		case services.ErrInvalidCredential:
			http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		default:
			logger.ErrorLogger.Printf("Sign-in error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Set auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    resp.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Hour * 24 * 7 / time.Second), // 7 days
	})

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.ErrorLogger.Printf("Error encoding response: %v", err)
	}
}

// SignUpHandler handles user registration with email/password
func (h *AuthHandler) SignUpHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check request method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var req models.SignUpRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		logger.ErrorLogger.Printf("Error parsing sign-up request: %v", err)
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	// Validate request
	if req.Email == "" || req.Password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Sign up user
	ctx := r.Context()
	resp, err := h.authService.SignUp(ctx, req)
	if err != nil {
		// Handle specific errors
		switch err {
		case services.ErrUserExists:
			http.Error(w, "User already exists", http.StatusConflict)
		default:
			logger.ErrorLogger.Printf("Sign-up error: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
		return
	}

	// Set auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    resp.AccessToken,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(time.Hour * 24 * 7 / time.Second), // 7 days
	})

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		logger.ErrorLogger.Printf("Error encoding response: %v", err)
	}
}
// OAuthSignInHandler handles sign-in/sign-up via OAuth providers
func (h *AuthHandler) OAuthSignInHandler(w http.ResponseWriter, r *http.Request) {
    // Set Content-Type header first for consistent responses
    w.Header().Set("Content-Type", "application/json")
    
    // Set CORS headers
    w.Header().Set("Access-Control-Allow-Origin", "*")
    w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
    w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

    // Handle preflight request
    if r.Method == "OPTIONS" {
        w.WriteHeader(http.StatusOK)
        return
    }

    // Check request method
    if r.Method != "POST" {
        w.WriteHeader(http.StatusMethodNotAllowed)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": true,
            "message": "Method not allowed",
        })
        return
    }

    // Get provider from URL path
    pathParts := strings.Split(r.URL.Path, "/")
    provider := ""
    for i, part := range pathParts {
        if part == "oauth" && i+1 < len(pathParts) {
            provider = pathParts[i+1]
            break
        }
    }

    logger.InfoLogger.Printf("OAuth provider from path: %s", provider)
    
    if provider == "" || provider == "unknown" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": true,
            "message": "Provider not specified",
        })
        return
    }
    
    // Parse request body
    var req models.OAuthRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        logger.ErrorLogger.Printf("Error parsing OAuth request: %v", err)
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": true,
            "message": "Invalid request format",
        })
        return
    }

    // Validate request
    if req.Code == "" {
        w.WriteHeader(http.StatusBadRequest)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "error": true,
            "message": "Code is required",
        })
        return
    }

    // Sign in user with OAuth
    ctx := r.Context()
    // resp, err := h.authService.OAuthSignIn(ctx, provider, req.Code, req.RedirectURI)
    // if err != nil {
    //     logger.ErrorLogger.Printf("OAuth error: %v", err)
    //     w.WriteHeader(http.StatusInternalServerError)
    //     json.NewEncoder(w).Encode(map[string]interface{}{
    //         "error": true,
    //         "message": fmt.Sprintf("OAuth authentication failed: %v", err),
    //     })
    //     return
    // }
	resp, err := h.authService.OAuthSignIn(ctx, provider, req.Code, req.RedirectURI)
	if err != nil {
		logger.ErrorLogger.Printf("OAuth error: %v", err)
		
		// Check for rate limiting error
		if strings.Contains(err.Error(), "429") || strings.Contains(err.Error(), "rate limit") {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": true,
				"message": "You've reached GitHub's rate limit. Please try again later.",
			})
			return
		}
		
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": true,
			"message": fmt.Sprintf("OAuth authentication failed: %v", err),
		})
		return
	}

    // Set auth cookie
    http.SetCookie(w, &http.Cookie{
        Name:     "auth_token",
        Value:    resp.AccessToken,
        Path:     "/",
        HttpOnly: true,
        Secure:   r.TLS != nil,
        SameSite: http.SameSiteLaxMode,
        MaxAge:   int(time.Hour * 24 * 7 / time.Second),
    })

    // Return response
    w.WriteHeader(http.StatusOK)
    json.NewEncoder(w).Encode(resp)
}

// OAuthURLHandler returns the URL for OAuth authentication
func (h *AuthHandler) OAuthURLHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	// Handle preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check request method
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get provider from URL path
	provider := r.URL.Path[len("/api/auth/oauth/url/"):]
	if provider == "" {
		http.Error(w, "Provider not specified", http.StatusBadRequest)
		return
	}

	// Get redirect URI from query parameter
	redirectURI := r.URL.Query().Get("redirect_uri")
	if redirectURI == "" {
		http.Error(w, "Redirect URI is required", http.StatusBadRequest)
		return
	}

	// Get OAuth URL
	url, err := h.authService.GetOAuthURL(provider, redirectURI)
	if err != nil {
		logger.ErrorLogger.Printf("Error getting OAuth URL: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get OAuth URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := map[string]string{"url": url}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		logger.ErrorLogger.Printf("Error encoding response: %v", err)
	}
}

// SignOutHandler handles user sign-out
func (h *AuthHandler) SignOutHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check request method
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Clear auth cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "auth_token",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1, // Delete cookie
	})

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]bool{"success": true})
}

// GetUserHandler returns the user profile for the authenticated user
func (h *AuthHandler) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

	// Handle preflight request
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// Check request method
	if r.Method != "GET" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get auth token from cookie or Authorization header
	var tokenString string
	cookie, err := r.Cookie("auth_token")
	if err == nil {
		tokenString = cookie.Value
	} else {
		// Try to get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			tokenString = authHeader[7:]
		}
	}

	if tokenString == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify token
	userID, err := h.authService.VerifyToken(tokenString)
	// backend/go/internal/api/handlers/auth.go (continued)
	
	if err != nil {
		logger.ErrorLogger.Printf("Token verification failed: %v", err)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user profile
	ctx := r.Context()
	user, err := h.authService.GetUserByID(ctx, userID)
	if err != nil {
		logger.ErrorLogger.Printf("Error getting user profile: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(user); err != nil {
		logger.ErrorLogger.Printf("Error encoding response: %v", err)
	}
}

// AuthMiddleware checks if the user is authenticated
func (h *AuthHandler) AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get auth token from cookie or Authorization header
		var tokenString string
		cookie, err := r.Cookie("auth_token")
		if err == nil {
			tokenString = cookie.Value
		} else {
			// Try to get token from Authorization header
			authHeader := r.Header.Get("Authorization")
			if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
				tokenString = authHeader[7:]
			}
		}

		if tokenString == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Verify token
		userID, err := h.authService.VerifyToken(tokenString)
		if err != nil {
			logger.ErrorLogger.Printf("Token verification failed: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user ID to request context
		ctx := context.WithValue(r.Context(), "userID", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}