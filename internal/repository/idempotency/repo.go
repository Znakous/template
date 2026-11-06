package idempotency

import (
	"github.com/Masterminds/squirrel"

	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

type Keys struct {
	database transaction.Executor
}

func New(database transaction.Executor) *Keys {
	return &Keys{database: database}
}