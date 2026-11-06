package statushistory

import (
	"github.com/Masterminds/squirrel"

	"github.com/Znakous/template/internal/infra/postgres/transaction"
)

var psql = squirrel.StatementBuilder.PlaceholderFormat(squirrel.Dollar)

type History struct {
	database transaction.Executor
}

func New(database transaction.Executor) *History {
	return &History{database: database}
}