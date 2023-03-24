package main

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	authenticationv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const (
	audience               = "factors"
	authorizationHeaderKey = "X-Auth-Token"
)

func main() {
	clientset, err := setupKubeClient()
	if err != nil {
		log.Fatalf("cannot create k8s clientset: %s", err)
	}

	srv := newServer(clientset)
	err = srv.start(":8080")
	if err != nil {
		log.Fatalf("cannot start server: %s", err)
	}
}

type Server struct {
	router    *gin.Engine
	clientset *kubernetes.Clientset
}

func newServer(clientset *kubernetes.Clientset) *Server {
	server := &Server{
		clientset: clientset,
	}
	server.setupRouter()
	return server
}

// setupKubeClient create k8s client object
func setupKubeClient() (*kubernetes.Clientset, error) {
	// Load kubeconfig from home file
	config, err := clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
	if err != nil {
		// Get in-cluster configuration
		inClusterConfig, err := rest.InClusterConfig()
		if err != nil {
			return nil, err
		}
		config = inClusterConfig
	}

	return kubernetes.NewForConfig(config)
}

// start run the http server
func (server *Server) start(addr string) error {
	return server.router.Run(addr)
}

// setupRouter use tokenReviewMiddleware and setup router
func (server *Server) setupRouter() {
	router := gin.Default()
	authRoutes := router.Group("/").Use(tokenReviewMiddleware(server.clientset))
	authRoutes.POST("/factor", server.factorHandler)
	server.router = router
}

type factorRequest struct {
	NR int64 `json:"nr" binding:"required,numeric,gt=0"`
}

type factorResponse struct {
	Factors []int64 `json:"factors"`
}

// factorHandler the parameter is obtained and the result is computed
func (server *Server) factorHandler(ctx *gin.Context) {
	var req factorRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, errorResponse(err))
		return
	}

	factors, err := factor(req.NR)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, errorResponse(err))
		return
	}

	resp := factorResponse{Factors: factors}
	ctx.JSON(http.StatusOK, resp)
}

// errorResponse wrapper errors
func errorResponse(err error) gin.H {
	return gin.H{"error": err.Error()}
}

// tokenReviewMiddleware validate the token
func tokenReviewMiddleware(clienset *kubernetes.Clientset) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		authorizationHeader := ctx.GetHeader(authorizationHeaderKey)
		if len(authorizationHeader) == 0 {
			err := errors.New("authorization header is not provided")
			ctx.AbortWithStatusJSON(http.StatusUnauthorized, errorResponse(err))
			return
		}

		if ok, err := tokenReviewRequest(clienset, ctx, authorizationHeader); !ok {
			ctx.AbortWithStatusJSON(http.StatusForbidden, errorResponse(err))
			return
		}

		ctx.Next()
	}
}

// tokenReviewRequest attempts to authenticate a token to a known user.
func tokenReviewRequest(clientset *kubernetes.Clientset, ctx *gin.Context, token string) (bool, error) {
	tokenReview := &authenticationv1.TokenReview{
		Spec: authenticationv1.TokenReviewSpec{
			Token: token,
		},
	}

	review, err := clientset.AuthenticationV1().TokenReviews().Create(ctx, tokenReview, metav1.CreateOptions{})
	audiences := review.Status.Audiences

	return review.Status.Authenticated && validateAudiences(audiences), err
}

// validateAudiences validate audience is in APIServer option `--api-audiences` values
func validateAudiences(audiences []string) bool {
	for _, v := range audiences {
		if v == audience {
			return true
		}
		continue
	}
	return false
}

// factor calculates all factors of a given positive integer nr.
// It uses an efficient approach based on prime factorization,
// which reduces the number of operations compared to checking every number in a loop.
// The function returns a slice of int64 containing the factors and an error if the input is not valid.
func factor(nr int64) ([]int64, error) {
	// Initialize an int64 slice with one element and an error check for positive integers
	fs := make([]int64, 1)
	if nr < 1 {
		return fs, errors.New("factors of 0 not computed, please provide a positive integer greater than 0")
	}

	fs[0] = 1

	// Helper function to append prime factors and their multiples
	apf := func(p int64, e int) {
		n := len(fs)
		for i, pp := 0, p; i < e; i, pp = i+1, pp*p {
			for j := 0; j < n; j++ {
				fs = append(fs, fs[j]*pp)
			}
		}
	}

	// Extract all factors of 2
	e := 0
	for ; nr&1 == 0; e++ {
		nr >>= 1
	}

	// Append factors of 2
	apf(2, e)

	// Extract and append other prime factors and their multiples
	for d := int64(3); nr > 1; d += 2 {
		// If d*d is greater than nr, set d to nr (it means nr is prime)
		if d*d > nr {
			d = nr
		}

		// Count the number of times nr is divisible by d
		for e = 0; nr%d == 0; e++ {
			nr /= d
		}

		// Append prime factors and their multiples
		if e > 0 {
			apf(d, e)
		}
	}

	return fs, nil
}
