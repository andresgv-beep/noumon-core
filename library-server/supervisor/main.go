package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	serviceName     = "NoumonServer"
	restartExitCode = 75
)

type supervisor struct {
	binDir string
	logDir string

	mu       sync.Mutex
	children map[string]*exec.Cmd
}

func main() {
	if len(os.Args) > 1 && os.Args[1] != "run" {
		if err := handleServiceCommand(os.Args[1]); err != nil {
			log.Fatal(err)
		}
		return
	}

	logPath, err := supervisorLogPath()
	if err == nil {
		if file, openErr := openRotatedLog(logPath, maxLogSize); openErr == nil {
			defer file.Close()
			log.SetOutput(file)
		}
	}

	s, err := newSupervisor()
	if err != nil {
		log.Fatal(err)
	}
	if err := runPlatform(s); err != nil {
		log.Fatal(err)
	}
}

func newSupervisor() (*supervisor, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	logDir, err := supervisorDataDir("logs")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	return &supervisor{binDir: filepath.Dir(exe), logDir: logDir, children: map[string]*exec.Cmd{}}, nil
}

func (s *supervisor) run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var workers sync.WaitGroup

	if translate := findExecutable(s.binDir, "translate-wrap"); translate != "" {
		workers.Add(1)
		go func() {
			defer workers.Done()
			s.runLoop(ctx, "translate", translate, s.translateEnv, false)
		}()
	}

	core := findExecutable(s.binDir, "library-server", "core")
	if core == "" {
		return fmt.Errorf("no se encontro library-server junto a %s", s.binDir)
	}
	s.runLoop(ctx, "core", core, s.coreEnv, true)
	cancel()
	workers.Wait()
	return nil
}

// restartDelay decide cuánto esperar antes de relanzar (sleep) y el backoff
// que queda armado para la siguiente caída (next). Un reinicio administrativo
// (código 75) siempre espera 300ms y rearma el backoff a 1s: se evalúa antes
// que el uptime para que una sesión larga no lo pise. Un proceso que aguantó
// más de 30s se considera sano y también rearma a 1s.
func restartDelay(current, uptime time.Duration, adminRestart bool) (sleep, next time.Duration) {
	if adminRestart {
		return 300 * time.Millisecond, time.Second
	}
	if uptime > 30*time.Second {
		current = time.Second
	}
	next = current * 2
	if next > 30*time.Second {
		next = 30 * time.Second
	}
	return current, next
}

func (s *supervisor) runLoop(ctx context.Context, name, executable string, processEnv func() []string, core bool) {
	delay := time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		started := time.Now()
		adminRestart := false
		cmd := exec.Command(executable)
		cmd.Dir = s.binDir
		// Se recalcula en cada arranque: los cambios guardados por el Panel se
		// aplican al reiniciar Core sin que la interfaz tenga que tocar el servicio.
		cmd.Env = processEnv()
		setChildAttributes(cmd)
		var output *os.File
		if file, err := s.processLog(name); err == nil {
			output = file
			cmd.Stdout, cmd.Stderr = output, output
		}
		if err := cmd.Start(); err != nil {
			log.Printf("%s no pudo arrancar: %v", name, err)
			if output != nil {
				output.Close()
			}
		} else {
			s.setChild(name, cmd)
			log.Printf("%s iniciado pid=%d", name, cmd.Process.Pid)
			wait := make(chan error, 1)
			go func() { wait <- cmd.Wait() }()
			select {
			case <-ctx.Done():
				stopProcessTree(cmd)
				select {
				case <-wait:
				case <-time.After(10 * time.Second):
					_ = cmd.Process.Kill()
					<-wait
				}
				s.clearChild(name, cmd)
				return
			case err := <-wait:
				s.clearChild(name, cmd)
				exitCode := commandExitCode(err)
				if core && exitCode == restartExitCode {
					log.Print("reinicio administrativo solicitado")
					adminRestart = true
				} else {
					log.Printf("%s termino (codigo=%d, uptime=%s): %v", name, exitCode, time.Since(started).Round(time.Second), err)
				}
			}
			if output != nil {
				output.Close()
			}
		}

		sleep, next := restartDelay(delay, time.Since(started), adminRestart)
		delay = next
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleep):
		}
	}
}

func (s *supervisor) setChild(name string, cmd *exec.Cmd) {
	s.mu.Lock()
	s.children[name] = cmd
	s.mu.Unlock()
}

func (s *supervisor) clearChild(name string, cmd *exec.Cmd) {
	s.mu.Lock()
	if s.children[name] == cmd {
		delete(s.children, name)
	}
	s.mu.Unlock()
}

// maxLogSize acota cada fichero de log. Con una sola generación .old el uso
// total queda en ~2× por proceso. La rotación ocurre al abrir (arranque del
// supervisor o relanzamiento del hijo): un hijo que viva meses sin caerse
// puede superarlo hasta su siguiente reinicio, porque el hijo escribe
// directamente sobre el descriptor y no se puede rotar por debajo en Windows.
const maxLogSize = 5 << 20 // 5 MB

// openRotatedLog abre path en modo append; si ya supera limit, lo aparta
// antes a path+".old" (sustituyendo la generación anterior).
func openRotatedLog(path string, limit int64) (*os.File, error) {
	if info, err := os.Stat(path); err == nil && info.Size() >= limit {
		_ = os.Remove(path + ".old")
		_ = os.Rename(path, path+".old")
	}
	return os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
}

func (s *supervisor) processLog(name string) (*os.File, error) {
	return openRotatedLog(filepath.Join(s.logDir, name+".log"), maxLogSize)
}

func (s *supervisor) coreEnv() []string {
	extra := map[string]string{"LIBRARY_SUPERVISED": "1"}
	stateRoot, stateErr := supervisorDataDir("data")
	configPath, configErr := supervisorConfigPath()
	var cfg supervisorConfig
	if configErr == nil {
		extra["NOUMON_LIBRARY_CONFIG"] = configPath
		var readErr error
		cfg, readErr = readSupervisorConfig(configPath)
		if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
			// Sin esto, un config.json corrupto degrada en silencio: contentRoot
			// desaparece y el pool vuelve al directorio de estado ("¿dónde está
			// mi biblioteca?") sin ninguna pista en el log.
			log.Printf("config.json ilegible, se usan valores por defecto: %v", readErr)
		}
	}

	// "Publicar en la red local" del Panel. Un BIND del entorno del operador
	// tiene la última palabra; sin él, lanAccess abre a toda la LAN.
	if os.Getenv("BIND") == "" && cfg.LanAccess {
		extra["BIND"] = "0.0.0.0"
	}

	// POOL_ROOT solo contiene biblioteca pesada. Las bases administrativas se
	// quedan en ProgramData para que desconectar el disco no borre usuarios ni
	// impida entrar al Panel.
	if stateErr == nil {
		extra["DB_PATH"] = filepath.Join(stateRoot, "db", "library.db")
		extra["DOWNLOADS_DB"] = filepath.Join(stateRoot, "db", "downloads.db")
	}
	if root := os.Getenv("POOL_ROOT"); root != "" {
		extra["POOL_ROOT"] = root
		extra["POOL_PROVIDER"] = env("POOL_PROVIDER", "environment")
		extra["NOUMON_LIBRARY_STORAGE_MANAGED"] = "environment"
	} else {
		root := stateRoot
		provider := "host"
		if cfg.ContentRoot != "" {
			root = cfg.ContentRoot
			provider = "configured"
		}
		if root != "" {
			extra["POOL_ROOT"] = root
		}
		extra["POOL_PROVIDER"] = provider
	}
	if os.Getenv("TRANSLATE_URL") == "" && findExecutable(s.binDir, "translate-wrap") != "" {
		extra["TRANSLATE_URL"] = "http://127.0.0.1:" + env("TRANSLATE_PORT", "8091")
	}
	return mergeEnv(os.Environ(), extra)
}

// supervisorConfig refleja el config.json que escribe el Panel a través del
// Core (storageConfig en core/storage.go): mismos campos, mismo fichero.
type supervisorConfig struct {
	ContentRoot string `json:"contentRoot"`
	LanAccess   bool   `json:"lanAccess"`
}

func supervisorConfigPath() (string, error) {
	root, err := supervisorDataDir("")
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "config.json"), nil
}

func readSupervisorConfig(path string) (supervisorConfig, error) {
	var cfg supervisorConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	err = json.Unmarshal(raw, &cfg)
	return cfg, err
}

func (s *supervisor) translateEnv() []string {
	extra := map[string]string{
		"PORT": env("TRANSLATE_PORT", "8091"),
		"BIND": "127.0.0.1",
	}
	if modelsDir := translateModelsDir(); modelsDir != "" {
		extra["MODELS_DIR"] = modelsDir
	}
	return mergeEnv(os.Environ(), extra)
}

// translateModelsDir mantiene los modelos dentro de la misma biblioteca pesada
// que muestra el Panel. MODELS_DIR permite una eleccion explicita para despliegues
// especiales, y POOL_ROOT conserva el comportamiento esperado por entorno.
func translateModelsDir() string {
	if root := os.Getenv("MODELS_DIR"); root != "" {
		return root
	}
	if root := os.Getenv("POOL_ROOT"); root != "" {
		return filepath.Join(root, "models")
	}
	if configPath, err := supervisorConfigPath(); err == nil {
		if cfg, err := readSupervisorConfig(configPath); err == nil && cfg.ContentRoot != "" {
			return filepath.Join(cfg.ContentRoot, "models")
		}
	}
	if stateRoot, err := supervisorDataDir("data"); err == nil {
		return filepath.Join(stateRoot, "models")
	}
	return ""
}

func mergeEnv(base []string, extra map[string]string) []string {
	for key, value := range extra {
		prefix := strings.ToUpper(key) + "="
		filtered := base[:0]
		for _, entry := range base {
			if !strings.HasPrefix(strings.ToUpper(entry), prefix) {
				filtered = append(filtered, entry)
			}
		}
		base = append(filtered, key+"="+value)
	}
	return base
}

func commandExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func findExecutable(dir string, names ...string) string {
	for _, name := range names {
		candidate := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			candidate += ".exe"
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func supervisorDataDir(child string) (string, error) {
	if root := os.Getenv("NOUMON_LIBRARY_DATA"); root != "" {
		return filepath.Join(root, child), nil
	}
	if runtime.GOOS == "windows" {
		root := os.Getenv("ProgramData")
		if root == "" {
			root = `C:\ProgramData`
		}
		return filepath.Join(root, "Noumon", child), nil
	}
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "noumon", child), nil
}

func supervisorLogPath() (string, error) {
	dir, err := supervisorDataDir("logs")
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "supervisor.log"), nil
}

func runConsole(s *supervisor) error {
	ctx, cancel := signalContext()
	defer cancel()
	return s.run(ctx)
}
