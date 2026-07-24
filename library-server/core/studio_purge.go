package main

import (
	"os"
	"path/filepath"
)

// purgeStudioContent permanently removes an explicitly archived work. The
// private asset directory is first moved aside so a failed database
// transaction can put it back; after commit, the quarantine is deleted.
func (s *Server) purgeStudioContent(id string, editor *User) error {
	s.studioPublishMu.Lock()
	defer s.studioPublishMu.Unlock()

	current, err := s.store.getStudioDocument(id, editor)
	if err != nil {
		return err
	}
	if current.Status != "archived" {
		return errStudioPurgeRequiresArchive
	}

	originalDir, quarantineDir, err := s.quarantineStudioAssets(id)
	if err != nil {
		return err
	}
	if err := s.store.purgeStudioDocument(id, editor); err != nil {
		if quarantineDir != "" {
			_ = os.Rename(quarantineDir, originalDir)
		}
		return err
	}
	if quarantineDir != "" {
		_ = os.RemoveAll(quarantineDir)
	}
	return nil
}

func (s *Server) quarantineStudioAssets(documentID string) (string, string, error) {
	if s.studioRoot == "" {
		return "", "", nil
	}
	dir, err := secureStudioAssetDir(s.studioRoot, documentID, false)
	if os.IsNotExist(err) {
		return "", "", nil
	}
	if err != nil {
		return "", "", err
	}
	quarantineID, err := newStudioID()
	if err != nil {
		return "", "", err
	}
	quarantine := filepath.Join(filepath.Dir(dir), ".purge-"+quarantineID)
	if err := os.Rename(dir, quarantine); err != nil {
		return "", "", err
	}
	return dir, quarantine, nil
}
