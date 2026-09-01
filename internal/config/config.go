package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	DBUrl             string
	DBMaxConns        int32
	DBMinConns        int32
	DBMaxConnLifetime time.Duration
	DBMaxConnIdleTime time.Duration
	RedisAddr         string
}

const (
	defaultDBMaxConns        int32 = 10
	defaultDBMinConns        int32 = 2
	defaultDBMaxConnLifetime       = time.Hour
	defaultDBMaxConnIdleTime       = 30 * time.Minute
)

func ReadConfig() (*Config, error) {
	err := godotenv.Load()
	if err != nil {
		return nil, fmt.Errorf("error loading .env file: %w", err)
	}
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")
	dbUrl := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable", dbUser, dbPassword, dbHost, dbPort, dbName)

	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	dbMaxConns, err := readInt32("DB_MAX_CONNS", defaultDBMaxConns)
	if err != nil {
		return nil, err
	}
	dbMinConns, err := readInt32("DB_MIN_CONNS", defaultDBMinConns)
	if err != nil {
		return nil, err
	}
	dbMaxConnLifetime, err := readDuration("DB_MAX_CONN_LIFETIME", defaultDBMaxConnLifetime)
	if err != nil {
		return nil, err
	}
	dbMaxConnIdleTime, err := readDuration("DB_MAX_CONN_IDLE_TIME", defaultDBMaxConnIdleTime)
	if err != nil {
		return nil, err
	}

	if dbMaxConns <= 0 {
		return nil, fmt.Errorf("DB_MAX_CONNS must be greater than zero")
	}
	if dbMinConns < 0 {
		return nil, fmt.Errorf("DB_MIN_CONNS must not be negative")
	}
	if dbMinConns > dbMaxConns {
		return nil, fmt.Errorf("DB_MIN_CONNS must not exceed DB_MAX_CONNS")
	}
	if dbMaxConnLifetime <= 0 {
		return nil, fmt.Errorf("DB_MAX_CONN_LIFETIME must be greater than zero")
	}
	if dbMaxConnIdleTime <= 0 {
		return nil, fmt.Errorf("DB_MAX_CONN_IDLE_TIME must be greater than zero")
	}

	return &Config{
		DBUrl:             dbUrl,
		DBMaxConns:        dbMaxConns,
		DBMinConns:        dbMinConns,
		DBMaxConnLifetime: dbMaxConnLifetime,
		DBMaxConnIdleTime: dbMaxConnIdleTime,
		RedisAddr:         redisAddr,
	}, nil
}

func readInt32(name string, defaultValue int32) (int32, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseInt(value, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return int32(parsed), nil
}

func readDuration(name string, defaultValue time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("invalid %s: %w", name, err)
	}
	return parsed, nil
}
