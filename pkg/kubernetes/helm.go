package kubernetes

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"text/template"

	piperhttp "github.com/SAP/jenkins-library/pkg/http"
	"github.com/SAP/jenkins-library/pkg/log"
)

// HelmExecutor is used for mock
type HelmExecutor interface {
	RunHelmUpgrade() error
	RunHelmLint() error
	RunHelmInstall() error
	RunHelmUninstall() error
	RunHelmTest() error
	RunHelmPublish() error
	RunHelmDependency() error
}

// HelmExecute struct
type HelmExecute struct {
	utils   DeployUtils
	config  HelmExecuteOptions
	verbose bool
	stdout  io.Writer
}

// HelmExecuteOptions struct holds common parameters for functions RunHelm...
type HelmExecuteOptions struct {
	ExecOpts                  ExecuteOptions
	AppTemplates              []string `json:"appTemplates,omitempty"`
	AppVersion                string   `json:"appVersion,omitempty"`
	CustomTLSCertificateLinks []string `json:"customTlsCertificateLinks,omitempty"`
	Dependency                string   `json:"dependency,omitempty" validate:"possible-values=build list update"`
	DumpLogs                  bool     `json:"dumpLogs,omitempty"`
	FilterTest                string   `json:"filterTest,omitempty"`
	HelmCommand               string   `json:"helmCommand,omitempty"`
	PackageDependencyUpdate   bool     `json:"packageDependencyUpdate,omitempty"`
	PublishVersion            string   `json:"publishVersion,omitempty"`
	TargetRepositoryURL       string   `json:"targetRepositoryURL,omitempty"`
	TargetRepositoryName      string   `json:"targetRepositoryName,omitempty"`
	TargetRepositoryUser      string   `json:"targetRepositoryUser,omitempty"`
	TargetRepositoryPassword  string   `json:"targetRepositoryPassword,omitempty"`
	Version                   string   `json:"version,omitempty"`
}

// NewHelmExecutor creates HelmExecute instance
func NewHelmExecutor(config HelmExecuteOptions, utils DeployUtils, verbose bool, stdout io.Writer) HelmExecutor {
	return &HelmExecute{
		config:  config,
		utils:   utils,
		verbose: verbose,
		stdout:  stdout,
	}
}

// runHelmInit is used to set up env for executing helm command
func (h *HelmExecute) runHelmInit() error {
	helmLogFields := map[string]interface{}{}
	helmLogFields["Chart Path"] = h.config.ExecOpts.ChartPath
	helmLogFields["Namespace"] = h.config.ExecOpts.Namespace
	helmLogFields["Deployment Name"] = h.config.ExecOpts.DeploymentName
	helmLogFields["Context"] = h.config.ExecOpts.KubeContext
	helmLogFields["Kubeconfig"] = h.config.ExecOpts.KubeConfig
	log.Entry().WithFields(helmLogFields).Debug("Calling Helm")

	helmEnv := []string{fmt.Sprintf("KUBECONFIG=%v", h.config.ExecOpts.KubeConfig)}

	log.Entry().Debugf("Helm SetEnv: %v", helmEnv)
	h.utils.SetEnv(helmEnv)
	h.utils.Stdout(h.stdout)

	return nil
}

// runHelmAdd is used to add a chart repository
func (h *HelmExecute) runHelmAdd() error {
	helmParams := []string{
		"repo",
		"add",
	}
	if len(h.config.TargetRepositoryName) == 0 {
		return fmt.Errorf("there is no TargetRepositoryName value. 'helm repo add' command requires 2 arguments")
	}
	if len(h.config.TargetRepositoryUser) != 0 {
		helmParams = append(helmParams, "--username", h.config.TargetRepositoryUser)
	}
	if len(h.config.TargetRepositoryPassword) != 0 {
		helmParams = append(helmParams, "--password", h.config.TargetRepositoryPassword)
	}
	helmParams = append(helmParams, h.config.TargetRepositoryName)
	helmParams = append(helmParams, h.config.TargetRepositoryURL)
	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm add call failed")
	}

	return nil
}

// RunHelmUpgrade is used to upgrade a release
func (h *HelmExecute) RunHelmUpgrade() error {
	if len(h.config.ExecOpts.ChartPath) == 0 {
		return fmt.Errorf("there is no ChartPath value. The chartPath value is mandatory")
	}

	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	if err := h.runHelmAdd(); err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"upgrade",
		h.config.ExecOpts.DeploymentName,
		h.config.ExecOpts.ChartPath,
	}

	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	for _, v := range h.config.ExecOpts.HelmValues {
		helmParams = append(helmParams, "--values", v)
	}

	helmParams = append(
		helmParams,
		"--install",
		"--namespace", h.config.ExecOpts.Namespace,
	)

	if h.config.ExecOpts.ForceUpdates {
		helmParams = append(helmParams, "--force")
	}

	helmParams = append(helmParams, "--wait", "--timeout", fmt.Sprintf("%vs", h.config.ExecOpts.HelmDeployWaitSeconds))

	if !h.config.ExecOpts.KeepFailedDeployments {
		helmParams = append(helmParams, "--atomic")
	}

	if len(h.config.ExecOpts.AdditionalParameters) > 0 {
		helmParams = append(helmParams, h.config.ExecOpts.AdditionalParameters...)
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm upgrade call failed")
	}

	return nil
}

// RunHelmLint is used to examine a chart for possible issues
func (h *HelmExecute) RunHelmLint() error {
	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"lint",
		h.config.ExecOpts.ChartPath,
	}

	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	h.utils.Stdout(h.stdout)
	log.Entry().Info("Calling helm lint ...")
	log.Entry().Debugf("Helm parameters: %v", helmParams)
	if err := h.utils.RunExecutable("helm", helmParams...); err != nil {
		log.Entry().WithError(err).Fatal("Helm lint call failed")
	}

	return nil
}

// RunHelmInstall is used to install a chart
func (h *HelmExecute) RunHelmInstall() error {
	if len(h.config.ExecOpts.ChartPath) == 0 {
		return fmt.Errorf("there is no ChartPath value. The chartPath value is mandatory")
	}

	if err := h.runHelmInit(); err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	if err := h.runHelmAdd(); err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"install",
		h.config.ExecOpts.DeploymentName,
		h.config.ExecOpts.ChartPath,
	}
	helmParams = append(helmParams, "--namespace", h.config.ExecOpts.Namespace)
	helmParams = append(helmParams, "--create-namespace")
	if !h.config.ExecOpts.KeepFailedDeployments {
		helmParams = append(helmParams, "--atomic")
	}
	helmParams = append(helmParams, "--wait", "--timeout", fmt.Sprintf("%vs", h.config.ExecOpts.HelmDeployWaitSeconds))
	for _, v := range h.config.ExecOpts.HelmValues {
		helmParams = append(helmParams, "--values", v)
	}
	if len(h.config.ExecOpts.AdditionalParameters) > 0 {
		helmParams = append(helmParams, h.config.ExecOpts.AdditionalParameters...)
	}
	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	if h.verbose {
		helmParamsDryRun := helmParams
		helmParamsDryRun = append(helmParamsDryRun, "--dry-run")
		if err := h.runHelmCommand(helmParamsDryRun); err != nil {
			log.Entry().WithError(err).Error("Helm install --dry-run call failed")
		}
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm install call failed")
	}

	return nil
}

// RunHelmUninstall is used to uninstall a chart
func (h *HelmExecute) RunHelmUninstall() error {
	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	if err := h.runHelmAdd(); err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"uninstall",
		h.config.ExecOpts.DeploymentName,
	}
	if len(h.config.ExecOpts.Namespace) <= 0 {
		return fmt.Errorf("namespace has not been set, please configure namespace parameter")
	}
	helmParams = append(helmParams, "--namespace", h.config.ExecOpts.Namespace)
	if h.config.ExecOpts.HelmDeployWaitSeconds > 0 {
		helmParams = append(helmParams, "--wait", "--timeout", fmt.Sprintf("%vs", h.config.ExecOpts.HelmDeployWaitSeconds))
	}
	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	if h.verbose {
		helmParamsDryRun := helmParams
		helmParamsDryRun = append(helmParamsDryRun, "--dry-run")
		if err := h.runHelmCommand(helmParamsDryRun); err != nil {
			log.Entry().WithError(err).Error("Helm uninstall --dry-run call failed")
		}
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm uninstall call failed")
	}

	return nil
}

// RunHelmPackage is used to package a chart directory into a chart archive
func (h *HelmExecute) runHelmPackage() error {
	if len(h.config.ExecOpts.ChartPath) == 0 {
		return fmt.Errorf("there is no ChartPath value. The chartPath value is mandatory")
	}

	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"package",
		h.config.ExecOpts.ChartPath,
	}
	if len(h.config.Version) > 0 {
		helmParams = append(helmParams, "--version", h.config.Version)
	}
	if h.config.PackageDependencyUpdate {
		helmParams = append(helmParams, "--dependency-update")
	}
	if len(h.config.AppVersion) > 0 {
		helmParams = append(helmParams, "--app-version", h.config.AppVersion)
	}
	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}
	if len(h.config.AppTemplates) > 0 {
		if err := h.runHelmWrite(); err != nil {
			return fmt.Errorf("failed to get values: %v", err)
		}
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm package call failed")
	}

	return nil
}

// RunHelmTest is used to run tests for a release
func (h *HelmExecute) RunHelmTest() error {
	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	helmParams := []string{
		"test",
		h.config.ExecOpts.ChartPath,
	}
	if len(h.config.FilterTest) > 0 {
		helmParams = append(helmParams, "--filter", h.config.FilterTest)
	}
	if h.config.DumpLogs {
		helmParams = append(helmParams, "--logs")
	}
	if h.verbose {
		helmParams = append(helmParams, "--debug")
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm test call failed")
	}

	return nil
}

// RunHelmDependency is used to manage a chart's dependencies
func (h *HelmExecute) RunHelmDependency() error {
	if len(h.config.Dependency) == 0 {
		return fmt.Errorf("there is no dependency value. Possible values are build, list, update")
	}

	helmParams := []string{
		"dependency",
	}

	helmParams = append(helmParams, h.config.Dependency)

	helmParams = append(helmParams, h.config.ExecOpts.ChartPath)

	if len(h.config.ExecOpts.AdditionalParameters) > 0 {
		helmParams = append(helmParams, h.config.ExecOpts.AdditionalParameters...)
	}

	if err := h.runHelmCommand(helmParams); err != nil {
		log.Entry().WithError(err).Fatal("Helm dependency call failed")
	}

	return nil
}

//RunHelmPublish is used to upload a chart to a registry
func (h *HelmExecute) RunHelmPublish() error {
	err := h.runHelmInit()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	err = h.runHelmPackage()
	if err != nil {
		return fmt.Errorf("failed to execute deployments: %v", err)
	}

	if len(h.config.TargetRepositoryURL) == 0 {
		return fmt.Errorf("there's no target repository for helm chart publishing configured")
	}

	repoClientOptions := piperhttp.ClientOptions{
		Username:     h.config.TargetRepositoryUser,
		Password:     h.config.TargetRepositoryPassword,
		TrustedCerts: h.config.CustomTLSCertificateLinks,
	}

	h.utils.SetOptions(repoClientOptions)

	binary := fmt.Sprintf("%v", h.config.ExecOpts.DeploymentName+"-"+h.config.PublishVersion+".tgz")

	targetPath := fmt.Sprintf("%v/%s", h.config.ExecOpts.DeploymentName, binary)

	separator := "/"

	if strings.HasSuffix(h.config.TargetRepositoryURL, "/") {
		separator = ""
	}

	targetURL := fmt.Sprintf("%s%s%s", h.config.TargetRepositoryURL, separator, targetPath)

	log.Entry().Infof("publishing artifact: %s", targetURL)

	response, err := h.utils.UploadRequest(http.MethodPut, targetURL, binary, "", nil, nil, "binary")
	if err != nil {
		return fmt.Errorf("couldn't upload artifact: %w", err)
	}

	if !(response.StatusCode == 200 || response.StatusCode == 201) {
		return fmt.Errorf("couldn't upload artifact, received status code %d", response.StatusCode)
	}

	return nil
}

func (h *HelmExecute) runHelmCommand(helmParams []string) error {

	h.utils.Stdout(h.stdout)
	log.Entry().Infof("Calling helm %v ...", h.config.HelmCommand)
	log.Entry().Debugf("Helm parameters: %v", helmParams)
	if err := h.utils.RunExecutable("helm", helmParams...); err != nil {
		log.Entry().WithError(err).Fatalf("Helm %v call failed", h.config.HelmCommand)
		return err
	}

	return nil
}

// runHelmWrite is used to write helm values to values.yaml file
func (h *HelmExecute) runHelmWrite() error {
	_, containerRegistry, err := splitRegistryURL(h.config.ExecOpts.ContainerRegistryURL)
	if err != nil {
		log.Entry().WithError(err).Fatalf("Container registry url '%v' incorrect", h.config.ExecOpts.ContainerRegistryURL)
	}

	helmValues, err := defineDeploymentValues(h.config.ExecOpts, containerRegistry)
	if err != nil {
		return fmt.Errorf("failed to process deployment values: %v", err)
	}

	for _, templateFile := range h.config.AppTemplates {
		err := renderTemplate(templateFile, helmValues, h.utils)
		if err != nil {
			return fmt.Errorf("failed to render template: %v", err)
		}
	}

	return nil
}

func renderTemplate(file string, deploymentValues *deploymentValues, utils DeployUtils) error {
	temp, err := utils.FileRead(file)
	if err != nil {
		log.Entry().WithError(err).Fatalf("Error when reading template '%v'", file)
	}

	re := regexp.MustCompile(`image:[ ]*<image-name>`)
	placeholderFound := re.Match(temp)

	if placeholderFound {
		log.Entry().Warn("image placeholder '<image-name>' is deprecated and does not support multi-image replacement, please use Helm-like template syntax '{{ .Values.image.[image-name].reposotory }}:{{ .Values.image.[image-name].tag }}")
		if deploymentValues.singleImage {
			// Update image name in deployment yaml, expects placeholder like 'image: <image-name>'
			temp = []byte(re.ReplaceAllString(string(temp), fmt.Sprintf("image: %s:%s", deploymentValues.get("image.repository"), deploymentValues.get("image.tag"))))
		} else {
			return fmt.Errorf("multi-image replacement not supported for single image placeholder")
		}
	}

	err = deploymentValues.mapValues()
	if err != nil {
		return fmt.Errorf("failed to map values using 'valuesMapping' configuration: %v", err)
	}

	buf := bytes.NewBufferString("")
	tpl, err := template.New("appTemplate").Parse(string(temp))
	if err != nil {
		return fmt.Errorf("failed to parse template file: %v", err)
	}

	err = tpl.Execute(buf, deploymentValues.asHelmValues())
	if err != nil {
		return fmt.Errorf("failed to render template file: %v", err)
	}

	err = utils.FileWrite(file, buf.Bytes(), 0700)
	if err != nil {
		return fmt.Errorf("Error when updating template '%v': %v", file, err)
	}

	return nil
}
