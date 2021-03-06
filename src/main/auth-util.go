package main

import (
	"github.com/dgrijalva/jwt-go"
	"github.com/globalsign/mgo/bson"
	"github.com/julienschmidt/httprouter"
	"golang.org/x/crypto/bcrypt"
	"interfaces-conv"
	"interfaces-internal"
	"net/http"
	"time"
)

// Account middleware

func WithAccountVerify(next accountPassRoute) httprouter.Handle {
	return func(w http.ResponseWriter, r *http.Request, params httprouter.Params) {
		w.Header().Set("Content-Type", "application/json")

		token := r.Header.Get("x-access-token")
		if token == "" {
			SendError(w, http.StatusUnauthorized, "No token provided.", 3000)
			return
		}

		claims, err := GetJWTClaims(token, SECRET) // verify token authenticity
		if err != nil {
			SendError(w, http.StatusInternalServerError, "Failed to authenticate token.", 3001)
			return
		}

		var result bson.M // Get account
		err = IAccountCollection.FindId(bson.ObjectIdHex(claims["id"].(string))).One(&result)
		if err != nil {
			if err.Error() == "not found" { // Check if account exists
				SendError(w, http.StatusInternalServerError, "Failed to authenticate token.", 3001)
			} else {
				if DEBUG {
					println(err)
				}
				SendError(w, http.StatusInternalServerError, internalServerError+" (Problem finding account)", 3002)
			}
			return
		}

		acc, err := interfaces_conv.ConvertBSONToIAccount(result)
		if err != nil {
			SendError(w, http.StatusInternalServerError, internalServerError+" (Problem finding account)", 3002)
			return
		}

		if acc.AuthKey != claims["authkey"] {
			SendError(w, http.StatusInternalServerError, "Failed to authenticate token.", 3001)
			return
		}

		if !acc.IsEmailVerified { // Check for email verification
			SendError(w, http.StatusUnauthorized, unauthorized+" (Email not verified.)", 3003)
			return
		}

		next(w, r, params, result) // call next middleware or main router function
	}
}

// Login route
// POST /v1/auth/login
// https://connectustoday.github.io/api-server/api-reference#login

func LoginRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")

	type requestForm struct {
		ID *string `json:"id" schema:"id"`
		Password *string `json:"password" schema:"password"`
	}

	var req requestForm
	err := DecodeRequest(r, &req)
	if err != nil { // Check decoding error
		SendError(w, http.StatusBadRequest, badRequest+" (Bad request.)", 4050)
		return
	}
	if !VerifyFieldsExist(&req, FormOmit([]string{}), false) { // Check request for correct fields
		SendError(w, http.StatusBadRequest, badRequest+" (Bad request.)", 4050)
		return
	}

	var account interfaces_internal.IAccount
	err = IAccountCollection.Find(GetOneAccountQuery(*req.ID)).One(&account) // find user in database
	if err != nil {
		if err.Error() == "not found" {
			SendError(w, http.StatusBadRequest, "Invalid login.", 3101)
		} else {
			SendError(w, http.StatusInternalServerError, internalServerError, 3100)
		}
		return
	}

	err = bcrypt.CompareHashAndPassword([]byte(account.Password), []byte(*req.Password))
	if err != nil { // check if password is valid
		SendError(w, http.StatusBadRequest, "Invalid login.", 3101)
		return
	}
	if !account.IsEmailVerified { // check if email is verified
		SendError(w, http.StatusBadRequest, badRequest+" (Email not verified.)", 3102)
		return
	}

	// generate jwt for client
	token := jwt.New(jwt.SigningMethodHS256)

	claims := make(jwt.MapClaims)
	claims["id"] = account.ID.Hex()
	claims["authkey"] = account.AuthKey
	claims["exp"] = time.Now().Add(time.Second * time.Duration(TOKEN_EXPIRY)).Unix()
	token.Claims = claims
	tokenString, err := token.SignedString([]byte(SECRET)) // sign with secret
	if err != nil {
		SendError(w, http.StatusInternalServerError, internalServerError, 3100)
		return
	}

	_, err = w.Write([]byte(`{"token": "` + tokenString + `"}`)) // return token to client

	if err != nil {
		SendError(w, http.StatusInternalServerError, internalServerError, 3100)
		return
	}
}

// Reset password route (after email)
// POST /v1/auth/reset-password
// https://connectustoday.github.io/api-server/api-reference#authentication

func EmailResetPasswordRoute(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	w.Header().Set("Content-Type", "application/json")

	type requestForm struct {
		Password *string `json:"password" schema:"password"`
		Token    *string `json:"token" schema:"token"`
	}

	var req requestForm
	err := DecodeRequest(r, &req)
	if err != nil { // Check decoding error
		SendError(w, http.StatusBadRequest, badRequest+" (Bad request.)", 4050)
		return
	}
	if !VerifyFieldsExist(&req, FormOmit([]string{}), false) { // Check request for correct fields
		SendError(w, http.StatusBadRequest, badRequest+" (Bad request.)", 4050)
		return
	}

	claims, err := GetJWTClaims(*req.Token, SECRET) // verify token authenticity
	if err != nil {
		SendError(w, http.StatusBadRequest, badRequest + " (Invalid token.)", 4001)
		return
	}

	var a bson.M
	err = IAccountCollection.FindId(bson.ObjectIdHex(claims["id"].(string))).One(&a)
	if CheckMongoQueryError(w, err, " (Account not found.)", 4002, 4000) != nil {
		return
	}

	acc, err := interfaces_conv.ConvertBSONToIAccount(a)
	if err != nil {
		SendError(w, http.StatusInternalServerError, internalServerError, 4000)
		return
	}

	if acc.PasswordResetToken != *req.Token {
		SendError(w, http.StatusBadRequest, badRequest + " (Invalid token.)", 4001)
		return
	}

	// Get hashed bcrypt password
	hashedPassword, err := CreateHashedPassword(*req.Password)
	if err != nil { // check for bcrypt error
		SendError(w, http.StatusInternalServerError, internalServerError+" (There was a problem registering the account.)", 3203)
		return
	}

	// Update account info
	if acc.Type == "User" {
		user, err := interfaces_conv.ConvertBSONToIUser(a)
		if err != nil {
			SendError(w, http.StatusInternalServerError, internalServerError, 4000)
			return
		}

		user.AuthKey = GenAuthKey() // invalidate ALL login tokens
		user.PasswordResetToken = ""
		user.Password = string(hashedPassword)

		err = IAccountCollection.Update(bson.M{"_id": bson.ObjectIdHex(claims["id"].(string))}, user)
		if err != nil {
			SendError(w, http.StatusInternalServerError, internalServerError, 4000)
			return
		}
	} else if acc.Type == "Organization" {
		org, err := interfaces_conv.ConvertBSONToIOrganization(a)
		if err != nil {
			SendError(w, http.StatusInternalServerError, internalServerError, 4000)
			return
		}

		org.AuthKey = GenAuthKey() // invalidate ALL login tokens
		org.PasswordResetToken = ""
		org.Password = string(hashedPassword)

		err = IAccountCollection.Update(bson.M{"_id": bson.ObjectIdHex(claims["id"].(string))}, org)
		if err != nil {
			SendError(w, http.StatusInternalServerError, internalServerError, 4000)
			return
		}
	}

	WriteOK(w)
}
