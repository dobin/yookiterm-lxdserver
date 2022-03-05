package main

import (
	"fmt"
	"net/http"

	"github.com/form3tech-oss/jwt-go"
)

func getUserId(r *http.Request) string {
	userContext := r.Context().Value("user")

	if userContext == nil {
		logger.Errorf("Authentication error")
		auditLog("", r, "Failed authentication")
		return ""
	}
	claims := userContext.(*jwt.Token).Claims.(jwt.MapClaims)
	userId := claims["userId"].(string)

	return userId
}

func userIsAdmin(r *http.Request) bool {
	userContext := r.Context().Value("user")
	if userContext == nil {
		logger.Errorf("Authentication error")
		auditLog("", r, "Admin interface: failed authentication")
		return false
	}

	claims := userContext.(*jwt.Token).Claims.(jwt.MapClaims)
	isAdmin := claims["admin"].(bool)

	return isAdmin
}

func jwtValidate(tokenString string) (bool, string) {
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Don't forget to validate the alg is what you expect:
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("Unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(config.Jwtsecret), nil
	})

	if err != nil {
		fmt.Println("FAIL1", err)
		return false, ""
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return true, claims["userId"].(string)
	} else {
		fmt.Println("Fail", claims)
		return false, ""
	}
}
