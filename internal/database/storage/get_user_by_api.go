package storage

import (
	"NewsAggregator/internal/model"
	"context"
)

func (repo *Repository) GetUserByApiKey(ctx context.Context, apiKey string) (model.User, error) {
	query := `SELECT * FROM users WHERE api_key = $1;`

	var user model.User
	err := repo.db.QueryRow(ctx, query, apiKey).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt, &user.Name, &user.ApiKey)
	if err != nil {
		return model.User{}, err
	}

	return user, nil
}
