package txpool

import (
	"database/sql"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

func initDB() (*sql.DB, error) {
	os.Remove("./transactions.db")
	db, err := sql.Open("sqlite3", "./transactions.db")
	if err != nil {
		return nil, err
	}
	defer db.Close()

	sqlStmt := `
	create table txs (hash BINARY(32) not null primary key, local BOOL, sender BINARY(20), nonce BIGINT, cost BIGINT, gasfeecap BIGINT, slots INT);
	delete from txs;
	`
	_, err = db.Exec(sqlStmt)
	if err != nil {
		return nil, err
	}
	return db, nil
}

func fillErr(errors []error, err error) []error {
	for i := 0; i < len(errors); i++ {
		errors[i] = err
	}
	return errors
}

func (t *TxPool) add(txs []*types.Transaction, local bool) []error {
	var (
		errors = make([]error, len(txs))
	)
	tx, err := t.db.Begin()
	if err != nil {
		return fillErr(errors, err)
	}
	stmt, err := tx.Prepare("insert into txs(hash, local, sender, nonce, cost, gasfeecap, slots) values(?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return fillErr(errors, err)
	}
	defer stmt.Close()
	for i, tx := range txs {
		// Check if we know the tx already
		if t.txs.Get(tx.Hash()) != nil {
			log.Trace("Discarding already known transaction", "hash", tx.Hash())
			errors[i] = err
			continue
		}
		// Validate the transaction
		if err := t.validateTx(tx, local); err != nil {
			errors[i] = err
			continue
		}

		sender, _ := t.signer.Sender(tx)
		// Check if the pool is full
		if uint64(numSlots(tx)+t.txs.Slots()) > t.config.GlobalSlots+t.config.GlobalQueue {
			if err := t.displaceTxs(tx, sender, local); err != nil {
				errors[i] = err
				continue
			}
		}

		// Insert the transaction into our db
		_, err = stmt.Exec(tx.Hash(), local, sender, tx.Nonce(), tx.Cost().Uint64(), tx.GasFeeCap().Uint64(), numSlots(tx))
		if err != nil {
			errors[i] = err
		}
		t.txs.Add(tx)
	}
	err = tx.Commit()
	if err != nil {
		return fillErr(errors, err)
	}

	return errors
}

func (t *TxPool) displaceTxs(tx *types.Transaction, sender common.Address, local bool) error {
	if !local {
		drop, err := t.isUnderpriced(tx)
		if err != nil {
			return err
		}
		if err := t.dropUnderpriced(drop); err != nil {
			return err
		}
	}
	return nil
}

func (t *TxPool) dropUnderpriced(hashes []common.Hash) error {
	for _, hash := range hashes {
		tx := t.txs.Remove(hash)
		_, err := t.db.Exec("DELETE FROM txs WHERE hash = ?", hash)
		if err != nil {
			return err
		}
		log.Trace("Discarding freshly underpriced transaction", "hash", hash, "gasTipCap", tx.GasTipCap(), "gasFeeCap", tx.GasFeeCap())
	}
	return nil
}

func (t *TxPool) isUnderpriced(tx *types.Transaction) ([]common.Hash, error) {
	rows, err := t.db.Query("SELECT hash, gasfeecap, slots FROM db SORTBY gasfeecap ASC LIMIT 20")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var (
		allSlots = 0
		hashes   []common.Hash
	)
	for rows.Next() {
		var (
			h      common.Hash
			feecap uint64
			slots  int
		)

		if err := rows.Scan(&h, &feecap, &slots); err != nil {
			return nil, err
		}
		if feecap > tx.GasFeeCap().Uint64() {
			log.Trace("Discarding underpriced transaction", "hash", tx.Hash(), "gasTipCap", tx.GasTipCap(), "gasFeeCap", tx.GasFeeCap())
			return nil, ErrUnderpriced
		}
		allSlots += slots
		if allSlots > numSlots(tx) {
			break
		}
		hashes = append(hashes, h)
	}
	return hashes, rows.Err()
}
