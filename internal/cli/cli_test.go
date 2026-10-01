package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

// testAddr é um endereço só do teste: o app instalado pode estar aberto e
// ocupando o pipe de verdade.
func testAddr(t *testing.T) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		return fmt.Sprintf(`\\.\pipe\hyphp-teste-%d-%d`, os.Getpid(), time.Now().UnixNano())
	}
	// Fora do t.TempDir: no macOS ele fica sob /var/folders/.../T/<nome do
	// teste>, e o caminho do socket passaria dos 104 bytes do sun_path (o
	// bind falha com "invalid argument").
	dir, err := os.MkdirTemp("/tmp", "hyphp")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir + "/s.sock"
}

func serveTest(t *testing.T, h http.Handler) string {
	t.Helper()
	addr := testAddr(t)
	l, err := Listen(addr)
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: h}
	go func() { _ = srv.Serve(l) }()
	t.Cleanup(func() { _ = srv.Close() })
	return addr
}

// Ida e volta pelo pipe de verdade: argumentos, cabeçalhos e resultado.
func TestCallPeloPipe(t *testing.T) {
	var gotCaller, gotArgv string
	var gotArgs ServiceArgs
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+CallPath+"{cmd}", func(w http.ResponseWriter, r *http.Request) {
		gotCaller, gotArgv = r.Header.Get(HeaderCaller), r.Header.Get(HeaderArgv)
		_ = json.NewDecoder(r.Body).Decode(&gotArgs)
		w.Header().Set(HeaderLang, "en")
		_ = json.NewEncoder(w).Encode([]Service{{ID: gotArgs.ID, State: "ready"}})
	})
	c := NewClient(serveTest(t, mux), "pwsh.exe")
	c.Argv = "start mysql"
	var out []Service
	if err := c.Call(context.Background(), CmdStart, ServiceArgs{ID: "mysql"}, &out); err != nil {
		t.Fatal(err)
	}
	if len(out) != 1 || out[0].ID != "mysql" || out[0].State != "ready" {
		t.Errorf("resultado = %+v", out)
	}
	if gotCaller != "pwsh.exe" || gotArgv != "start mysql" || c.Lang != "en" {
		t.Errorf("cabeçalhos: caller %q argv %q lang %q", gotCaller, gotArgv, c.Lang)
	}
}

// O erro do app chega com a mensagem dele (já traduzida) e o status.
func TestCallErroDoApp(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+CallPath+"{cmd}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ErrorBody{Error: "restart precisa de: id"})
	})
	err := NewClient(serveTest(t, mux), "").Call(context.Background(), CmdRestart, nil, nil)
	var re *RemoteError
	if !errors.As(err, &re) || re.Status != http.StatusBadRequest || re.Message != "restart precisa de: id" {
		t.Fatalf("err = %v (%T)", err, err)
	}
}

// Sem app escutando, a CLI precisa distinguir "fechado" de erro qualquer:
// é o que vira o código de saída 3 e a dica de abrir o app.
func TestCallSemAppAberto(t *testing.T) {
	err := NewClient(testAddr(t), "").Call(context.Background(), CmdStatus, nil, nil)
	if !errors.Is(err, ErrAppNotRunning) {
		t.Fatalf("err = %v, want ErrAppNotRunning", err)
	}
}

// logs -f: as linhas chegam enquanto a resposta continua aberta.
func TestLogsStreaming(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST "+LogsPath, func(w http.ResponseWriter, r *http.Request) {
		fl := w.(http.Flusher)
		for i := range 3 {
			fmt.Fprintf(w, "linha %d\n", i)
			fl.Flush()
			time.Sleep(20 * time.Millisecond)
		}
	})
	var got []string
	err := NewClient(serveTest(t, mux), "").Logs(context.Background(), LogsArgs{ID: "mysql", Follow: true}, func(l string) {
		got = append(got, l)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, "|") != "linha 0|linha 1|linha 2" {
		t.Errorf("linhas = %q", got)
	}
}
