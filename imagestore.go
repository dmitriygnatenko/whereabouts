package main

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// filesDir — каталог на диске, куда сохраняются файлы фотографий вещей.
// В БД (item_images.url) хранится только ссылка на файл, не его содержимое.
const filesDir = "web/files"

// filesURLPrefix — префикс URL, по которому эти файлы отдаются наружу
// (см. регистрацию "/files/" в registerRoutes).
const filesURLPrefix = "/files/"

// ensureFilesDir создаёт каталог для файлов фотографий, если его ещё нет.
func ensureFilesDir() error {
	return os.MkdirAll(filesDir, 0o755)
}

// isStoredImageURL проверяет, что строка уже ссылается на ранее сохранённый
// файл, а не является свежим data:-URL с фронтенда, который ещё нужно
// обработать и сохранить.
func isStoredImageURL(s string) bool {
	return strings.HasPrefix(s, filesURLPrefix)
}

// saveImageFile пишет байты изображения в filesDir под случайным именем
// и возвращает URL, по которому файл будет доступен.
func saveImageFile(data []byte, ext string) (string, error) {
	if err := ensureFilesDir(); err != nil {
		return "", fmt.Errorf("failed to create files directory: %w", err)
	}
	name, err := randomFileName(ext)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(filesDir, name), data, 0o644); err != nil {
		return "", fmt.Errorf("failed to write image file: %w", err)
	}
	return filesURLPrefix + name, nil
}

// deleteImageFile удаляет файл, на который ссылается url — если он вообще
// указывает в наш каталог файлов. Отсутствующий файл не считается ошибкой:
// он мог быть уже удалён.
func deleteImageFile(url string) {
	if !isStoredImageURL(url) {
		return
	}
	name := strings.TrimPrefix(url, filesURLPrefix)
	// Защита от path traversal — имя файла не должно содержать разделителей пути.
	if name == "" || strings.ContainsAny(name, `/\`) {
		return
	}
	_ = os.Remove(filepath.Join(filesDir, name))
}

func randomFileName(ext string) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate file name: %w", err)
	}
	return hex.EncodeToString(b) + ext, nil
}

// migrateBase64ImagesToFiles переносит фотографии, сохранённые до перехода
// на файловое хранилище (когда item_images.url ещё хранил целиком data:-URL),
// в файлы на диске и переписывает ссылку в БД. На новых записях эта функция
// ничего не делает — они уже создаются как ссылки на файлы.
func migrateBase64ImagesToFiles(db *sql.DB) error {
	rows, err := db.Query(`SELECT id, url FROM item_images WHERE url LIKE 'data:%'`)
	if err != nil {
		return err
	}
	type legacyRow struct {
		id  int64
		url string
	}
	var toMigrate []legacyRow
	for rows.Next() {
		var lr legacyRow
		if err := rows.Scan(&lr.id, &lr.url); err != nil {
			rows.Close()
			return err
		}
		toMigrate = append(toMigrate, lr)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(toMigrate) == 0 {
		return nil
	}
	log.Printf("переношу %d старых фото в файлы...", len(toMigrate))

	for _, lr := range toMigrate {
		_, raw, err := parseDataURL(lr.url)
		if err != nil {
			log.Printf("предупреждение: не удалось разобрать старое фото (item_images.id=%d): %v", lr.id, err)
			continue
		}
		newURL, err := saveImageFile(raw, ".jpg")
		if err != nil {
			log.Printf("предупреждение: не удалось сохранить старое фото в файл (item_images.id=%d): %v", lr.id, err)
			continue
		}
		if _, err := db.Exec(`UPDATE item_images SET url = ? WHERE id = ?`, newURL, lr.id); err != nil {
			log.Printf("предупреждение: не удалось обновить ссылку на фото (item_images.id=%d): %v", lr.id, err)
		}
	}
	return nil
}
