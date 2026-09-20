package infrastructure

import (
	"fmt"

	"github.com/elastic/go-elasticsearch/v9"
)

type ESConfig struct {
	Addresses []string `mapstructure:"ES_ADDRESSES"` // Viper 會自動解析逗號分隔字串為 Slice
	Username  string   `mapstructure:"ES_USERNAME"`
	Password  string   `mapstructure:"ES_PASSWORD"`
}

func NewElasticsearch(cfg *ESConfig) (*elasticsearch.Client, error) {
	client, err := elasticsearch.NewClient(elasticsearch.Config{
		Addresses: cfg.Addresses,
		Username:  cfg.Username,
		Password:  cfg.Password,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	// 驗證連線狀況
	res, err := client.Info()
	if err != nil {
		return nil, fmt.Errorf("failed to connect to elasticsearch: %w", err)
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("elasticsearch ping error: %s", res.String())
	}

	return client, nil
}
