package store

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"arca/internal/store/models"
)

type Store struct {
	db *gorm.DB
}

// checkWritable reports an unwritable data directory in those words. SQLite
// answers the same situation with "unable to open database file: out of memory",
// which sends people looking for a memory problem that is not there.
func checkWritable(directory string) error {
	probe, err := os.CreateTemp(directory, ".write-probe-*")
	if err != nil {
		if user := os.Getuid(); user >= 0 {
			return fmt.Errorf("%s is not writable by uid %d: %w", directory, user, err)
		}
		return fmt.Errorf("%s is not writable: %w", directory, err)
	}

	name := probe.Name()
	probe.Close()
	return os.Remove(name)
}

func Open(path string) (*Store, error) {
	if directory := filepath.Dir(path); directory != "" {
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return nil, fmt.Errorf("create database directory: %w", err)
		}
		if err := checkWritable(directory); err != nil {
			return nil, err
		}
	}

	pragmas := url.Values{}
	pragmas.Add("_pragma", "journal_mode(WAL)")
	pragmas.Add("_pragma", "busy_timeout(5000)")
	pragmas.Add("_pragma", "foreign_keys(1)")
	pragmas.Add("_pragma", "synchronous(NORMAL)")

	db, err := gorm.Open(sqlite.Open(fmt.Sprintf("file:%s?%s", path, pragmas.Encode())), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	pool, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("access connection pool: %w", err)
	}
	pool.SetMaxOpenConns(8)
	pool.SetMaxIdleConns(8)
	pool.SetConnMaxLifetime(time.Hour)

	store := &Store{db: db}
	if err := store.migrate(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) migrate() error {
	if err := s.db.AutoMigrate(&models.Migration{}); err != nil {
		return err
	}
	if err := s.runMigrations(); err != nil {
		return err
	}

	return s.db.AutoMigrate(
		&models.Setting{},
		&models.User{},
		&models.Identity{},
		&models.Session{},
		&models.APIToken{},
		&models.Category{},
		&models.Repository{},
		&models.RepositoryAlias{},
		&models.Grant{},
		&models.RepositoryMember{},
		&models.DeploymentLink{},
		&models.Component{},
		&models.DistTag{},
		&models.Asset{},
		&models.RemoteMiss{},
		&models.TrafficEvent{},
		&models.DockerUpload{},
		&models.DockerManifest{},
		&models.DockerReference{},
	)
}

func (s *Store) Close() error {
	pool, err := s.db.DB()
	if err != nil {
		return err
	}
	return pool.Close()
}

func (s *Store) Transaction(work func(*Store) error) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		return work(&Store{db: tx})
	})
}

// query starts a statement with no inherited conditions, which matters inside
// transactions where the same handle is reused for several statements.
func (s *Store) query() *gorm.DB {
	return s.db.Session(&gorm.Session{NewDB: true})
}

func NowMillis() int64 { return time.Now().UnixMilli() }

func NewID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		panic("store: unable to read random bytes: " + err.Error())
	}
	return base64.RawURLEncoding.EncodeToString(buffer)
}

func isNotFound(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }
