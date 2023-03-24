package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

func main() {
	srv := newServer()
	err := srv.start(":8080")
	if err != nil {
		log.Fatalf("cannot start server: %s", err)
	}
}

type Server struct {
	router *gin.Engine
}

func newServer() *Server {
	server := &Server{}
	server.setupRouter()
	return server
}

func (server *Server) start(addr string) error {
	return server.router.Run(addr)
}

func (server *Server) setupRouter() {
	router := gin.Default()
	router.POST("/factor", server.reqWithToken)
	server.router = router
}

type factorRequest struct {
	NR int64 `json:"nr" binding:"required,numeric,gt=0"`
}

type factorResponse struct {
	Factors []int64 `json:"factors"`
}

func (server *Server) reqWithToken(ctx *gin.Context) {
	var freq factorRequest
	if err := ctx.ShouldBindJSON(&freq); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	// Read the service token
	token, err := readToken()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	body, err := json.Marshal(freq)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	client := &http.Client{}
	req, err := http.NewRequest("POST", "http://factor-server:8080/factor", bytes.NewBuffer(body))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
	}

	req.Header.Set("X-Auth-Token", string(token))
	serverResp, err := client.Do(req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}
	defer serverResp.Body.Close()

	if serverResp.StatusCode == http.StatusForbidden {
		err := errors.New("the HTTP request was not authenticated, downstream service responded with 403")
		ctx.JSON(http.StatusForbidden, errorResponse(err))
		return
	}

	if serverResp.StatusCode == http.StatusOK {
		respBody, err := io.ReadAll(serverResp.Body)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}

		var factResp factorResponse
		err = json.Unmarshal(respBody, &factResp)
		if err != nil {
			ctx.JSON(http.StatusInternalServerError, errorResponse(err))
			return
		}
		ctx.JSON(http.StatusOK, factResp)
	}
}

func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

func readToken() ([]byte, error) {
	file, err := os.Open("/var/run/secrets/tokens/factor-token")
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return io.ReadAll(file)
}
