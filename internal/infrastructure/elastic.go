package infrastructure

import (
	"context"
	"fmt"

	"github.com/elastic/go-elasticsearch/v8"
)

type ESConfig struct {
	Addresses []string `mapstructure:"addresses"` // Viper 會自動解析逗號分隔字串為 Slice
	Username  string   `mapstructure:"username"`
	Password  string   `mapstructure:"password"`
}

func NewElasticsearch(cfg *ESConfig) (*elasticsearch.TypedClient, error) {

	client, err := elasticsearch.NewTyped(
		elasticsearch.WithAddresses(cfg.Addresses...),
		elasticsearch.WithBasicAuth(cfg.Username, cfg.Password),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create elasticsearch client: %w", err)
	}

	// 使用 Ping 驗證連線
	if _, err := client.Ping().Do(context.Background()); err != nil {
		return nil, fmt.Errorf("elasticsearch ping failed: %w", err)
	}

	// 	// 驗證連線狀況 (TypedClient 的 Info 需要傳入 context)
	// 	res, err := client.Info().Do(context.Background())
	// 	if err != nil {
	// 		return nil, fmt.Errorf("failed to connect to elasticsearch: %w", err)
	// 	}
	//
	// 	// 可選：列印 Cluster 版本確認連線成功
	// 	_ = res.Version.Number

	return client, nil
}
