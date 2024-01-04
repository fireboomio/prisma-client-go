package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/joho/godotenv"

	"github.com/prisma/prisma-client-go/binaries/platform"
	"github.com/prisma/prisma-client-go/logger"
)

func (e *QueryEngine) Connect() error {
	logger.Debug.Printf("ensure query engine binary...")

	_ = godotenv.Load("e2e.env")
	_ = godotenv.Load("db/e2e.env")
	_ = godotenv.Load("prisma/e2e.env")

	startEngine := time.Now()

	file, err := e.ensure()
	if err != nil {
		return fmt.Errorf("ensure: %w", err)
	}

	if err := e.spawn(file); err != nil {
		return fmt.Errorf("spawn: %w", err)
	}

	logger.Debug.Printf("connecting took %s", time.Since(startEngine))
	logger.Debug.Printf("connected.")

	return nil
}

func (e *QueryEngine) Disconnect() error {
	e.disconnected = true
	logger.Debug.Printf("disconnecting...")

	if platform.Name() == "windows" {
		if err := e.cmd.Process.Kill(); err != nil {
			return fmt.Errorf("kill process: %w", err)
		}
		return nil
	}

	if err := e.cmd.Process.Signal(os.Interrupt); err != nil {
		return fmt.Errorf("send signal: %w", err)
	}

	if err := e.cmd.Wait(); err != nil {
		if err.Error() != "signal: interrupt" {
			return fmt.Errorf("wait for process: %w", err)
		}
	}

	logger.Debug.Printf("disconnected.")
	return nil
}

func (e *QueryEngine) ensure() (string, error) {

	// fireboom 直接从环境变量读
	prismaQueryEngineBinary := os.Getenv("PRISMA_QUERY_ENGINE_BINARY")
	if prismaQueryEngineBinary == "" {
		return "", fmt.Errorf("should set env var PRISMA_QUERY_ENGINE_BINARY ")
	}

	if _, err := os.Stat(prismaQueryEngineBinary); err != nil {
		return "", fmt.Errorf("PRISMA_QUERY_ENGINE_BINARY was provided, but no query engine was found at %s", prismaQueryEngineBinary)
	}

	return prismaQueryEngineBinary, nil
}

func (e *QueryEngine) spawn(file string) error {
	port, err := getPort()
	if err != nil {
		return fmt.Errorf("get free port: %w", err)
	}

	logger.Debug.Printf("running query-engine on port %s", port)

	e.url = "http://localhost:" + port

	e.cmd = exec.Command(file, "-p", port, "--enable-raw-queries")

	e.cmd.Stdout = os.Stdout
	e.cmd.Stderr = os.Stderr

	e.cmd.Env = append(
		os.Environ(),
		"PRISMA_DML="+e.Schema,
		"RUST_LOG=error",
		"RUST_LOG_FORMAT=json",
		"PRISMA_CLIENT_ENGINE_TYPE=binary",
	)

	// TODO fine tune this using log levels
	if logger.Enabled {
		e.cmd.Env = append(
			e.cmd.Env,
			"PRISMA_LOG_QUERIES=y",
			"RUST_LOG=info",
		)
	}

	logger.Debug.Printf("starting engine...")

	if err := e.cmd.Start(); err != nil {
		return fmt.Errorf("start command: %w", err)
	}

	logger.Debug.Printf("connecting to engine...")

	ctx := context.Background()

	// send a basic readiness healthcheck and retry if unsuccessful
	var connectErr error
	var gqlErrors []GQLError
	for i := 0; i < 100; i++ {
		body, err := e.Request(ctx, "GET", "/status", map[string]interface{}{})
		if err != nil {
			connectErr = err
			logger.Debug.Printf("could not connect; retrying... {}", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		var response GQLResponse

		if err := json.Unmarshal(body, &response); err != nil {
			connectErr = err
			logger.Debug.Printf("could not unmarshal response; retrying...")
			time.Sleep(50 * time.Millisecond)
			continue
		}

		if response.Errors != nil {
			gqlErrors = response.Errors
			logger.Debug.Printf("could not connect due to gql errors; retrying...")
			time.Sleep(50 * time.Millisecond)
			continue
		}

		connectErr = nil
		gqlErrors = nil
		break
	}

	if connectErr != nil {
		return fmt.Errorf("readiness query error: %w", connectErr)
	}

	if gqlErrors != nil {
		return fmt.Errorf("readiness gql errors: %+v", gqlErrors)
	}

	return nil
}
