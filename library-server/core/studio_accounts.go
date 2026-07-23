package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

var (
	errStudioDeletionStrategyRequired = errors.New("studio deletion strategy required")
	errStudioDeletionStrategyInvalid  = errors.New("studio deletion strategy invalid")
	errStudioTransferTargetInvalid    = errors.New("studio transfer target invalid")
	errStudioTransferQuota            = errors.New("studio transfer quota exceeded")
)

type StudioDeletionImpact struct {
	Documents int `json:"documents"`
	Published int `json:"published"`
	Assets    int `json:"assets"`
}

func studioDeletionImpactTx(tx *sql.Tx, userID int64) (StudioDeletionImpact, error) {
	var impact StudioDeletionImpact
	err := tx.QueryRow(`
		SELECT
			COUNT(*),
			COALESCE(SUM(CASE WHEN published_revision IS NOT NULL AND status!='archived' THEN 1 ELSE 0 END), 0)
		FROM studio_documents WHERE owner_user_id=?`, userID).
		Scan(&impact.Documents, &impact.Published)
	if err != nil {
		return impact, err
	}
	if err := tx.QueryRow(`
		SELECT COUNT(*) FROM studio_assets
		WHERE owner_user_id=? AND state!='deleted'`, userID).Scan(&impact.Assets); err != nil {
		return impact, err
	}
	return impact, nil
}

func (s *Store) deleteUserWithStudioStrategy(
	userID int64,
	username string,
	actor *User,
	strategy string,
	transferTo int64,
) (StudioDeletionImpact, error) {
	var impact StudioDeletionImpact
	tx, err := s.db.Begin()
	if err != nil {
		return impact, err
	}
	defer tx.Rollback()

	impact, err = studioDeletionImpactTx(tx, userID)
	if err != nil {
		return impact, err
	}
	if impact.Documents > 0 && strategy == "" {
		return impact, errStudioDeletionStrategyRequired
	}
	if impact.Documents == 0 {
		strategy = ""
	} else if strategy != "transfer" && strategy != "custody" && strategy != "withdraw" {
		return impact, errStudioDeletionStrategyInvalid
	}

	var newOwner *int64
	if strategy == "transfer" {
		if transferTo < 1 || transferTo == userID {
			return impact, errStudioTransferTargetInvalid
		}
		var isAdmin int
		if err := tx.QueryRow(`SELECT is_admin FROM users WHERE id=?`, transferTo).Scan(&isAdmin); err != nil {
			return impact, errStudioTransferTargetInvalid
		}
		quota := int64(2 << 30)
		if isAdmin == 0 {
			var canAuthor int
			if err := tx.QueryRow(`
				SELECT can_author, quota_bytes FROM user_capabilities
				WHERE user_id=?`, transferTo).Scan(&canAuthor, &quota); err != nil || canAuthor != 1 {
				return impact, errStudioTransferTargetInvalid
			}
			if quota <= 0 {
				quota = 2 << 30
			}
		}
		var sourceBytes, targetBytes int64
		if err := tx.QueryRow(`
			SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
			WHERE owner_user_id=? AND state!='deleted'`, userID).Scan(&sourceBytes); err != nil {
			return impact, err
		}
		if err := tx.QueryRow(`
			SELECT COALESCE(SUM(size_bytes), 0) FROM studio_assets
			WHERE owner_user_id=? AND state!='deleted'`, transferTo).Scan(&targetBytes); err != nil {
			return impact, err
		}
		if targetBytes+sourceBytes > quota {
			return impact, errStudioTransferQuota
		}
		value := transferTo
		newOwner = &value
	}

	documents, err := studioDocumentsOwnedByTx(tx, userID)
	if err != nil {
		return impact, err
	}
	now := time.Now().Unix()
	for _, document := range documents {
		oldRevision := document.Revision
		document.OwnerUserID = newOwner
		document.Revision++
		document.Updated = now
		if strategy == "withdraw" {
			document.Status = "archived"
			document.PublishedRevision = nil
			document.PublicationKind = ""
			document.PublicationTarget = ""
			document.Published = nil
		}
		snapshot, err := json.Marshal(document)
		if err != nil {
			return impact, err
		}
		var result sql.Result
		if strategy == "withdraw" {
			result, err = tx.Exec(`
				UPDATE studio_documents SET
					owner_user_id=NULL, status='archived', published_revision=NULL,
					publication_kind=NULL, publication_target=NULL,
					published_plain_text='', published=NULL, revision=?, updated=?
				WHERE id=? AND revision=?`,
				document.Revision, now, document.ID, oldRevision)
		} else {
			var owner any
			if newOwner != nil {
				owner = *newOwner
			}
			result, err = tx.Exec(`
				UPDATE studio_documents SET owner_user_id=?, revision=?, updated=?
				WHERE id=? AND revision=?`,
				owner, document.Revision, now, document.ID, oldRevision)
		}
		if err != nil {
			return impact, err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return impact, errStudioConflict
		}
		if _, err := tx.Exec(`
			INSERT INTO studio_revisions
				(document_id, revision, editor_user_id, editor_label, snapshot_json, created)
			VALUES (?,?,?,?,?,?)`,
			document.ID, document.Revision, actor.ID, actor.Username, string(snapshot), now); err != nil {
			return impact, err
		}
	}

	var assetOwner any
	if newOwner != nil {
		assetOwner = *newOwner
	}
	if _, err := tx.Exec(`
		UPDATE studio_assets SET owner_user_id=?
		WHERE owner_user_id=?`, assetOwner, userID); err != nil {
		return impact, err
	}
	if _, err := tx.Exec(`DELETE FROM studio_publish_targets WHERE user_id=?`, userID); err != nil {
		return impact, err
	}
	if _, err := tx.Exec(`DELETE FROM user_capabilities WHERE user_id=?`, userID); err != nil {
		return impact, err
	}
	if err := deleteUserDataTx(tx, username); err != nil {
		return impact, err
	}
	result, err := tx.Exec(`DELETE FROM users WHERE id=?`, userID)
	if err != nil {
		return impact, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return impact, sql.ErrNoRows
	}
	if err := tx.Commit(); err != nil {
		return impact, err
	}
	return impact, nil
}

func studioDocumentsOwnedByTx(tx *sql.Tx, userID int64) ([]StudioDocument, error) {
	rows, err := tx.Query(`
		SELECT `+studioDocumentColumns+`
		FROM studio_documents WHERE owner_user_id=?
		ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	var documents []StudioDocument
	for rows.Next() {
		document, err := scanStudioDocument(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		documents = append(documents, document)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	return documents, nil
}
