package main

import (
	"bytes"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"text/template"
	"time"

	"github.com/josexy/kubedebug/internal/config"
	"github.com/josexy/kubedebug/internal/reconciler"
	"github.com/josexy/logx"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	clientconfig "sigs.k8s.io/controller-runtime/pkg/client/config"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/manager/signals"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

var scheme = runtime.NewScheme()

var (
	loggerContext = logx.NewLogContext().
			WithLevel(logx.LevelTrace).
			WithLevelKey(true, logx.LevelOption{LowerKey: false}).
			WithColorfulset(true, logx.TextColorAttri{}).
			WithTimeKey(true, logx.TimeOption{Formatter: func(t time.Time) any { return t.Format("2006/01/02 15:04:05.000") }}).
			WithEscapeQuote(true).
			WithReflectValue(true).
			WithWriter(os.Stdout).
			WithEncoder(logx.Console)
	logger = loggerContext.Build()
)

func init() {
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
}

var (
	configPath string
	onlyWatch  bool
)

func main() {
	flag.StringVar(&configPath, "c", "config.yaml", "config file path")
	flag.BoolVar(&onlyWatch, "w", false, "only watch resource and not reconcile")
	flag.Parse()

	if err := config.ReadAndValidateConfigFile(configPath); err != nil {
		logger.Fatalf("load config failed: %v", err)
	}

	if err := generateVSCodeLaunchJsonFile(); err != nil {
		logger.Errorf("generate vscode launch config file failed: %v", err)
	}

	logf.SetLogger(zap.New())

	cacheByObjectMap := make(map[client.Object]cache.ByObject)
	var cacheByObject cache.ByObject
	var cacheConfig cache.Config
	if len(config.GetConfig().LabelSelector) > 0 {
		cacheConfig.LabelSelector = labels.SelectorFromSet(config.GetConfig().LabelSelector)
	}
	if len(config.GetConfig().FieldSelector) > 0 {
		cacheConfig.FieldSelector = fields.SelectorFromSet(config.GetConfig().FieldSelector)
	}

	var cacheObject bool
	if cacheConfig.LabelSelector != nil || cacheConfig.FieldSelector != nil {
		cacheObject = true
		cacheByObject.Namespaces = map[string]cache.Config{
			config.GetConfig().Namespace: cacheConfig,
		}
	}

	switch config.GetConfig().Type {
	case config.DeploymentType:
		cacheByObjectMap[&appsv1.Deployment{}] = cacheByObject
	case config.StatefulsetType:
		cacheByObjectMap[&appsv1.StatefulSet{}] = cacheByObject
	}

	mgr, err := manager.New(clientconfig.GetConfigOrDie(), manager.Options{
		Scheme:                        scheme,
		Metrics:                       server.Options{BindAddress: "0"}, // disable metrics server
		LeaderElection:                true,
		LeaderElectionID:              "e99848b9.debug-k8s-pod.com",
		LeaderElectionNamespace:       config.GetConfig().Namespace, // specify namespace if not running in cluster
		LeaderElectionReleaseOnCancel: true,
		Cache:                         cache.Options{ByObject: cacheByObjectMap},
	})
	if err != nil {
		logger.Error("could not create manager", logx.Error("error", err))
		os.Exit(1)
	}

	if cacheObject {
		switch config.GetConfig().Type {
		case config.DeploymentType:
			if err := (&reconciler.DeploymentReconciler{
				Scheme: mgr.GetScheme(),
				Client: mgr.GetClient(),
				CommonConfig: &reconciler.CommonConfig{
					OnlyWatch: onlyWatch,
					Logger:    logger.With(logx.String("controller", "deployment")),
				},
			}).Setup(mgr); err != nil {
				logger.Error("could not create controller", logx.Error("error", err))
				os.Exit(1)
			}
		case config.StatefulsetType:
			if err := (&reconciler.StatefulsetReconciler{
				Scheme: mgr.GetScheme(),
				Client: mgr.GetClient(),
				CommonConfig: &reconciler.CommonConfig{
					OnlyWatch: onlyWatch,
					Logger:    logger.With(logx.String("controller", "statefulset")),
				},
			}).Setup(mgr); err != nil {
				logger.Error("could not create controller", logx.Error("error", err))
				os.Exit(1)
			}
		}
	}

	if err := mgr.Start(signals.SetupSignalHandler()); err != nil {
		logger.Error("could not start manager", logx.Error("error", err))
		os.Exit(1)
	}
}

func generateVSCodeLaunchJsonFile() error {
	rawTemplate := `{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug K8S Pod Container",
      "type": "go",
      "request": "attach",
      "mode": "remote",
      "showLog": true,
      "host": "{{ .NodeHost }}",
      "port": {{ .NodePort }},
      "remotePath": "{{ .ProjectRootDir }}"
    }
  ]
}
`
	templ, err := template.New("launch").Parse(rawTemplate)
	if err != nil {
		return err
	}
	vscodeDir := filepath.Join(config.GetConfig().ProjectRootDir, ".vscode")
	if _, err = os.Stat(vscodeDir); errors.Is(err, os.ErrNotExist) {
		os.Mkdir(vscodeDir, 0755)
	}
	launchJson := filepath.Join(vscodeDir, "launch.json")
	if _, err = os.Stat(launchJson); err == nil {
		os.Rename(launchJson, launchJson+".bak")
	}
	buffer := bytes.NewBuffer(nil)
	if err = templ.Execute(buffer, config.GetConfig()); err != nil {
		return err
	}
	return os.WriteFile(launchJson, buffer.Bytes(), 0644)
}
