package app

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/mosteligible/mcp-codemode/coderunner/config"
	"github.com/mosteligible/mcp-codemode/coderunner/constants"
	"github.com/mosteligible/mcp-codemode/coderunner/core/common"
	"github.com/mosteligible/mcp-codemode/coderunner/core/handlers"
	"github.com/mosteligible/mcp-codemode/coderunner/core/types"
	workerclient "github.com/mosteligible/mcp-codemode/coderunner/core/worker_client"
	"github.com/mosteligible/mcp-codemode/coderunner/middlewares"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"google.golang.org/protobuf/types/known/emptypb"
)

type App struct {
	wrapper       http.Handler
	port          string
	appConfig     *config.Config
	redisClient   *redis.Client
	requestClient *http.Client
	workerClients *workerclient.WorkerConnections
}

func NewApp(port string) *App {
	conf := config.NewConfig()
	redisOpts := &redis.Options{
		Addr: conf.RedisHost + ":" + conf.RedisPort,
		DB:   conf.RedisDB,
	}
	if conf.RedisUser != "" {
		redisOpts.Username = conf.RedisUser
	}
	if conf.RedisPassword != "" {
		redisOpts.Password = conf.RedisPassword
	}
	slog.Info("starting redis client")
	redisClient := redis.NewClient(redisOpts)
	redisCtx, redisCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer redisCancel()
	if err := redisClient.Ping(redisCtx).Err(); err != nil {
		slog.Error("could not connect to redis", "addr", redisOpts.Addr, "error", err.Error())
	}

	slog.Info("remote hosts: " + strings.Join(conf.RemoteHosts, ", "))
	workerClients := workerclient.NewWorkerConnections(conf, redisClient)

	app := &App{
		port:        port,
		appConfig:   conf,
		redisClient: redisClient,
		requestClient: &http.Client{
			Timeout:   180 * time.Second,
			Transport: otelhttp.NewTransport(http.DefaultTransport),
		},
		workerClients: workerClients,
	}

	app.init()
	return app
}

func (a *App) init() {
	mux := http.NewServeMux()

	mux.HandleFunc("/run", a.RunCode)
	mux.HandleFunc("GET /proxy/{path...}", a.Proxy)
	mux.HandleFunc("POST /proxy/{path...}", a.Proxy)
	mux.HandleFunc("/status", a.status)
	handler := otelhttp.NewHandler(
		mux,
		"http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.Pattern
		}),
	)
	a.wrapper = middlewares.LoggingMiddleware(handler)
}

func (a *App) Start() error {
	return http.ListenAndServe(
		a.port, a.wrapper,
	)
}

func (a *App) getGrpcConnection(ctx context.Context, sessionId string) (*workerclient.WorkerClient, error) {
	if a.workerClients == nil {
		return nil, fmt.Errorf("no available worker connections")
	}
	return a.workerClients.GetWorker(ctx, sessionId)
}

func (a *App) status(w http.ResponseWriter, r *http.Request) {
	res, err := a.getGrpcConnection(r.Context(), "")
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "no available worker connections"})
		return
	}

	_, err = res.Client.Status(r.Context(), &emptypb.Empty{})
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "error connecting to worker"})
		return
	}
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]int16{"status": 200})
}

func (a *App) RunCode(w http.ResponseWriter, r *http.Request) {
	var codeRequest types.CodeRunnerRequest

	err := json.NewDecoder(r.Body).Decode(&codeRequest)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage("invalid request body"))
		return
	}

	conn, err := a.getGrpcConnection(r.Context(), codeRequest.SessionId)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage("no available worker connections"))
		return
	}
	output := common.ExecuteCommand(r.Context(), conn, codeRequest.Code, codeRequest.Language, codeRequest.SessionId)

	w.Header().Set("Content-Type", "application/json")
	if output.ErrorMessage != "" {
		w.WriteHeader(http.StatusInternalServerError)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	json.NewEncoder(w).Encode(output)
}

func (a *App) Proxy(w http.ResponseWriter, r *http.Request) {
	correlationID := uuid.New().String()
	proxyId := r.Header.Get(constants.PROXY_HEADER_KEY)
	proxyId = strings.TrimSpace(proxyId)
	if proxyId == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage("missing proxy ID"))
		return
	}

	target, err := common.GetTarget(r, correlationID)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage(
			fmt.Sprintf("Invalid error path - %s", err.Error()),
		))
		return
	}
	slog.Info(
		"sending proxy request",
		"url", target.Url,
		"method", target.Method,
		"correlation_id", correlationID,
	)

	token := a.redisClient.Get(r.Context(), proxyId)
	if token.Err() != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage("invalid proxy id"))
		return
	}
	target.Token = token.Val()

	apiResponse, err := handlers.RunProxyRequest(r.Context(), target, a.requestClient, correlationID)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(common.GetErrorResponseMessage("error processing proxy request"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(apiResponse)
}
