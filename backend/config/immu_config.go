package config

import "time"

type ConfigKey struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Environment string `json:"environment"`
	Category    string `json:"category"`
	Description string `json:"description"`
	IsEncrypted bool   `json:"is_encrypted"`
	CreatedAt   time.Time `json:"created_at"`
	CreatedBy   string    `json:"created_by"`
	UpdatedAt   time.Time `json:"updated_at"`
	UpdatedBy   string    `json:"updated_by"`
}

type ApplicationConfig struct {

	//Database configuration
	DBHost    string 
	DBPort    string
	DBUser    string 
	DBPassword string 
	DBName    string	
	DBSSLMode string

	//Redis configuration
	RedisHost     string
	RedisPort     string
	RedisPassword string
	RedisDB       int

	//JWT configuration
	JWTSecretKey string
	JWTExpiry    int

	//CORS configuration
	CORSAllowedOrigins []string
	CORSAllowedMethods []string
	CORSAllowedHeaders []string

	//APP configuration
	AppEnv        string
	AppPort       string
}