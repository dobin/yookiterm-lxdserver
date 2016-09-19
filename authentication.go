package main

import(
//  "github.com/joho/godotenv"
  "github.com/dgrijalva/jwt-go"
  "github.com/auth0/go-jwt-middleware"
	"net/http"
//  "time"
	"fmt"
//	"encoding/json"
  "github.com/gorilla/context"
)


func getUserId(r *http.Request) string {
  userContext := context.Get(r, "user")

  claims := userContext.(*jwt.Token).Claims.(jwt.MapClaims)
  userId := claims["userId"].(string)

  return userId
}


var jwtMiddleware = jwtmiddleware.New(jwtmiddleware.Options{
  ValidationKeyGetter: func(token *jwt.Token) (interface{}, error) {
    return ([]byte(config.Jwtsecret)), nil
  },
  SigningMethod: jwt.SigningMethodHS256,
})


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
