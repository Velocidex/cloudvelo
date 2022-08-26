package elastic_datastore

type ElasticConfiguration struct {
	Username           string   `json:"username"`
	Password           string   `json:"password"`
	APIKey             string   `json:"api_key"`
	Addresses          []string `json:"addresses"`
	CloudID            string   `json:"cloud_id"`
	DisableSSLSecurity bool     `json:"disable_ssl_security"`
	RootCerts          string   `json:"root_cert"`

	// The name of the index we should use (default velociraptor)
	Index string `json:"index"`

	// AWS S3 settings
	AWSRegion         string `json:"aws_region"`
	CredentialsKey    string `json:"credentials_key"`
	CredentialsSecret string `json:"credentials_secret"`
	Endpoint          string `json:"endpoint"`
	NoVerifyCert      bool   `json:"no_verify_cert"`
	Bucket            string `json:"bucket"`

	// Redis hostname and port
	RedisHost                      string `json:"redis_host"`
	RedisLocalCacheSize            int    `json:"redis_local_cache_size"`
	RedisLocalCacheDurationMinutes int    `json:"redis_local_cache_duration_minutes"`
	RedisCacheDurationMinutes      int    `json:"redis_cache_duration_minutes"`

	PublicKeyBucket string `json:"public_key_bucket"`

	LogoffRedirectURL string `json:"logoff_redirect_url"`
	DispatchUrl       string `json:"dispatch_url"`
}
