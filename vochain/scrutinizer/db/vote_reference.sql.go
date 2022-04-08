// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.13.0
// source: vote_reference.sql

package scrutinizerdb

import (
	"context"
	"database/sql"
	"time"

	"go.vocdoni.io/dvote/types"
)

const createVoteReference = `-- name: CreateVoteReference :execresult
INSERT INTO vote_references (
	nullifier, process_id, height, weight,
	tx_index, creation_time
) VALUES (
	?, ?, ?, ?,
	?, ?
)
`

type CreateVoteReferenceParams struct {
	Nullifier    types.Nullifier
	ProcessID    types.ProcessID
	Height       int64
	Weight       string
	TxIndex      int64
	CreationTime time.Time
}

func (q *Queries) CreateVoteReference(ctx context.Context, arg CreateVoteReferenceParams) (sql.Result, error) {
	return q.db.ExecContext(ctx, createVoteReference,
		arg.Nullifier,
		arg.ProcessID,
		arg.Height,
		arg.Weight,
		arg.TxIndex,
		arg.CreationTime,
	)
}

const getVoteReference = `-- name: GetVoteReference :one
SELECT nullifier, process_id, height, weight, tx_index, creation_time FROM vote_references
WHERE nullifier = ?
LIMIT 1
`

func (q *Queries) GetVoteReference(ctx context.Context, nullifier types.Nullifier) (VoteReference, error) {
	row := q.db.QueryRowContext(ctx, getVoteReference, nullifier)
	var i VoteReference
	err := row.Scan(
		&i.Nullifier,
		&i.ProcessID,
		&i.Height,
		&i.Weight,
		&i.TxIndex,
		&i.CreationTime,
	)
	return i, err
}