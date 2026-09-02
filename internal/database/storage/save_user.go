package storage

import (
	"context"
	"time"

	"NewsAggregator/internal/model"

	"github.com/google/uuid"
)

func (repo *Repository) SaveUser(ctx context.Context, id uuid.UUID, name, apiKey string) (model.User, error) {
	query := `
    INSERT INTO users (id, created_at, updated_at, name, api_key) 
    VALUES ($1, $2, $3, $4, $5) 
    RETURNING id, created_at, updated_at, name, api_key
	`

	var newUser model.User

	err := repo.db.QueryRow(ctx, query, id, time.Now().UTC(), time.Now().UTC(), name, apiKey).Scan(
		&newUser.ID, &newUser.CreatedAt, &newUser.UpdatedAt, &newUser.Name, &newUser.ApiKey)
	if err != nil {
		return model.User{}, err
	}

	return newUser, nil
}
