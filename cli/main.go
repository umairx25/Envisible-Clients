package main

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	_ "embed"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"golang.org/x/term"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultAPIURL       = "https://api.envisible.dev"
	defaultDashboardURL = "https://envisible.dev"
	pollWaitTimeout     = 120 * time.Second
	defaultPollDelay    = 5 * time.Second
	defaultRepo         = "umairx25/Envisible-Clients"
	cliBinaryName       = "envis"
)

//go:embed MANUAL.md
var manual string

type Config struct {
	BaseURL      string
	DashboardURL string
	APIVersion   string
	SessionPath  string
	CIToken      string
	ProjectPath  string
}

type Session struct {
	AccessToken  string                 `json:"access_token"`
	RefreshToken string                 `json:"refresh_token"`
	ExpiresAt    int64                  `json:"expires_at"`
	ExpiresIn    int                    `json:"expires_in,omitempty"`
	TokenType    string                 `json:"token_type,omitempty"`
	User         map[string]interface{} `json:"user"`
}

type SecretListResponse struct {
	Secrets []string `json:"secrets"`
}

type SecretValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type SecretValuesResponse struct {
	ProjectID string        `json:"project_id"`
	Secrets   []SecretValue `json:"secrets"`
}

type Project struct {
	ProjectID  string `json:"project_id"`
	Name       string `json:"name"`
	CITokenSet bool   `json:"ci_token_set"`
	Role       string `json:"role"`
}

type CurrentUser struct {
	ID string `json:"id"`
}

type CurrentUserProfile struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type Invite struct {
	ID             string `json:"id"`
	ProjectID      string `json:"project_id"`
	InviterID      string `json:"inviter_id"`
	RecipientEmail string `json:"recipient_email"`
	Status         string `json:"status"`
	CreatedAt      string `json:"created_at"`
	InviterName    string `json:"inviter_name"`
	InviterEmail   string `json:"inviter_email"`
	ProjectName    string `json:"project_name"`
}

type AuditCreateRequest struct {
	Action  string `json:"action"`
	Message string `json:"message"`
}

type SecretGetResponse struct {
	Value string `json:"value"`
}

type SecretUpsertRequest struct {
	Value string `json:"value"`
}

type SecretsBatchRequest struct {
	Names []string `json:"names"`
}

type CiTokenVerifyRequest struct {
	Token string `json:"token"`
}

type InviteCreateRequest struct {
	Email string `json:"email"`
	Role  string `json:"role,omitempty"`
}

type ProjectCreateRequest struct {
	Name string `json:"name"`
}

type ProjectRenameRequest struct {
	NewName string `json:"new_name"`
}

type ProjectMember struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
	Role  string `json:"role"`
}

type ProjectMembersResponse struct {
	ProjectID string          `json:"project_id"`
	Members   []ProjectMember `json:"members"`
}

type CiTokenResponse struct {
	Status string `json:"status"`
	Token  string `json:"token"`
}

type CiTokenVerifyResponse struct {
	Status bool `json:"status"`
}

type PollAuthResponse struct {
	IsAuth  bool            `json:"is_auth"`
	Content json.RawMessage `json:"content"`
	Detail  string          `json:"detail"`
}

type ProjectResolutionOptions struct {
	Prompt      string
	AllowCreate bool
	AllowManual bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	_ = loadDotenvFromCWD()

	if len(args) == 0 {
		printUsage()
		return nil
	}

	cfg, err := newConfig()
	if err != nil {
		return err
	}

	switch args[0] {
	case "help", "--help", "-h":
		if len(args) > 1 {
			printManualSection(args[1])
			return nil
		}
		printManualSection("Commands")
		fmt.Println()
		fmt.Println("Run `envis help <command>` for command details.")
		fmt.Println("Run `envis man` to show the full manual.")
		return nil
	case "man":
		printManual()
		return nil
	case "pull":
		return runPull(cfg, args[1:])
	case "push":
		return runPush(cfg, args[1:])
	case "secret-names":
		return runSecretNames(cfg, args[1:])
	case "secret-get":
		return runSecretGet(cfg, args[1:])
	case "secret-set":
		return runSecretSet(cfg, args[1:])
	case "secret-delete":
		return runSecretDelete(cfg, args[1:])
	case "projects":
		return runProjects(cfg, args[1:])
	case "project-set":
		return runProjectSet(cfg, args[1:])
	case "project-create":
		return runProjectCreate(cfg, args[1:])
	case "project-rename":
		return runProjectRename(cfg, args[1:])
	case "project-delete":
		return runProjectDelete(cfg, args[1:])
	case "project-members":
		return runProjectMembers(cfg, args[1:])
	case "project-member-remove":
		return runProjectMemberRemove(cfg, args[1:])
	case "get-many":
		return runGetMany(cfg, args[1:])
	case "invites":
		return runInvites(cfg, args[1:])
	case "invite-respond":
		return runInviteRespond(cfg, args[1:])
	case "invite-create":
		return runInviteCreate(cfg, args[1:])
	case "status":
		return runStatus(cfg, args[1:])
	case "ci-token-generate":
		return runCiTokenGenerate(cfg, args[1:])
	case "ci-token-reset":
		return runCiTokenReset(cfg, args[1:])
	case "ci-token-verify":
		return runCiTokenVerify(cfg, args[1:])
	case "login":
		if canPrompt(cfg) {
			ok, err := promptConfirm("Open browser to authenticate?", true)
			if err != nil {
				return err
			}
			if !ok {
				return errors.New("login canceled")
			}
		}
		_, err := ensureSession(cfg)
		if err != nil {
			return err
		}
		fmt.Println("✓ Logged in successfully.")
		return nil
	case "logout":
		if canPrompt(cfg) {
			ok, err := promptConfirm("Are you sure you want to log out?", false)
			if err != nil {
				return err
			}
			if !ok {
				fmt.Println("Logout canceled.")
				return nil
			}
		}
		if err := os.Remove(cfg.SessionPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errors.New("already logged out")
			}
			return fmt.Errorf("failed to remove session: %w", err)
		}
		fmt.Println("✓ Logged out successfully.")
		return nil
	case "update":
		return runUpdate(args[1:])
	case "uninstall":
		return runUninstall(args[1:])
	default:
		printUsage()
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Println("envis - Envault CLI")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  envis pull [--project-id <uuid>] [--output .env]")
	fmt.Println("  envis push [--project-id <uuid>] [--file .env]")
	fmt.Println("  envis secret-names [--project-id <uuid>]")
	fmt.Println("  envis secret-get [--project-id <uuid>] [--name <key>]")
	fmt.Println("  envis secret-set [--project-id <uuid>] [--name <key>] [--value <value>]")
	fmt.Println("  envis secret-delete [--project-id <uuid>] [--name <key>]")
	fmt.Println("  envis projects")
	fmt.Println("  envis project-set [--project-id <uuid>]")
	fmt.Println("  envis project-create [--name <name>]")
	fmt.Println("  envis project-rename [--project-id <uuid>] [--name <new-name>]")
	fmt.Println("  envis project-delete [--project-id <uuid>]")
	fmt.Println("  envis project-members [--project-id <uuid>]")
	fmt.Println("  envis project-member-remove [--project-id <uuid>] [--user-id <uuid>]")
	fmt.Println("  envis get-many [--project-id <uuid>] <secret1> <secret2> ...")
	fmt.Println("  envis invites")
	fmt.Println("  envis invite-respond [--invite-id <uuid>] [--accept|--reject]")
	fmt.Println("  envis invite-create [--project-id <uuid>] [--email <user@example.com>] [--role admin|member]")
	fmt.Println("  envis status")
	fmt.Println("  envis ci-token-generate [--project-id <uuid>]")
	fmt.Println("  envis ci-token-reset [--project-id <uuid>]")
	fmt.Println("  envis ci-token-verify [--project-id <uuid>] [--token <token>]")
	fmt.Println("  envis login")
	fmt.Println("  envis logout")
	fmt.Println("  envis update")
	fmt.Println("  envis uninstall")
	fmt.Println("  envis help")
	fmt.Println("  envis man")
}

func printManual() {
	fmt.Println(strings.TrimSpace(manual))
}

func printManualSection(section string) {
	if section == "" {
		printManual()
		return
	}
	needle := strings.ToLower(strings.TrimSpace(section))
	lines := strings.Split(manual, "\n")
	var out []string
	inSection := false
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			name := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(line, "## ")))
			if inSection {
				break
			}
			if name == needle {
				inSection = true
				out = append(out, line)
				continue
			}
		}
		if inSection {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		fmt.Printf("Unknown help topic: %s\n", section)
		fmt.Println("Run `envis help` to see available topics.")
		return
	}
	fmt.Println(strings.TrimSpace(strings.Join(out, "\n")))
}

func runPull(cfg Config, args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	output := fs.String("output", ".env", "Output env file path")
	noEnvExample := fs.Bool("no-env-example", false, "Disable .env.example population")
	if err := fs.Parse(args); err != nil {
		return err
	}
	outputProvided := flagWasPassed(fs, "output")

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{
		AllowManual: true,
	})
	if err != nil {
		return err
	}

	if !outputProvided && canPrompt(cfg) {
		selected, err := promptText("Output file", *output, true)
		if err != nil {
			return err
		}
		*output = selected
	}

	secrets, err := getAllSecrets(client, cfg, projectID)
	if err != nil {
		return err
	}

	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})

	existingKeys, existingValues, err := parseEnvFile(*output)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		existingKeys = nil
		existingValues = nil
	}

	values := make(map[string]string, len(existingValues)+len(secrets))
	for key, value := range existingValues {
		values[key] = value
	}

	secretKeySet := make(map[string]bool, len(secrets))
	secretKeys := make([]string, 0, len(secrets))
	for _, secret := range secrets {
		if strings.TrimSpace(secret.Name) == "" {
			continue
		}
		secretKeySet[secret.Name] = true
		secretKeys = append(secretKeys, secret.Name)
		values[secret.Name] = secret.Value
	}

	extraKeys := make([]string, 0, len(existingKeys))
	for _, key := range existingKeys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if !secretKeySet[key] {
			extraKeys = append(extraKeys, key)
		}
	}

	keys := append(secretKeys, extraKeys...)

	var buf bytes.Buffer
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		buf.WriteString(key)
		buf.WriteString("=")
		buf.WriteString(formatEnvValue(values[key]))
		buf.WriteString("\n")
	}

	if err := os.WriteFile(*output, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", *output, err)
	}

	if err := ensureGitignoreForOutput(*output); err != nil {
		return err
	}

	if !*noEnvExample {
		added, err := updateEnvExample(envExamplePath(*output), keys)
		if err != nil {
			return err
		}
		if len(added) == 0 {
			fmt.Println("No new vars added to .env.example.")
		} else {
			fmt.Printf("Added %d var(s) to .env.example: %s\n", len(added), strings.Join(added, ", "))
		}
	}

	if cfg.CIToken == "" {
		message := fmt.Sprintf("Pulled %d secrets via CLI pull (%s)", len(secrets), *output)
		if err := createAuditEvent(client, cfg, projectID, "pulled", message); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write audit event: %v\n", err)
		}
	}

	successf("Wrote %d secret(s) to %s", len(secrets), *output)
	return nil
}

func runGetMany(cfg Config, args []string) error {
	fs := flag.NewFlagSet("get-many", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	rawNames := fs.Args()
	if len(rawNames) == 0 {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("secret names", "pass names as positional arguments")
		}
		line, err := promptText("Secret names (space separated)", "", true)
		if err != nil {
			return err
		}
		rawNames = strings.Fields(line)
	}

	names := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return errors.New("get-many requires non-empty secret names")
		}
		names = append(names, trimmed)
	}

	secrets, err := getBatchSecrets(client, cfg, projectID, names)
	if err != nil {
		return err
	}

	values := make(map[string]string, len(secrets))
	for _, secret := range secrets {
		if strings.TrimSpace(secret.Name) == "" {
			continue
		}
		values[secret.Name] = secret.Value
	}

	for _, name := range names {
		value, ok := values[name]
		if !ok {
			return fmt.Errorf("secret not found: %s", name)
		}
		fmt.Printf("%s=%s\n", name, formatEnvValue(value))
	}

	return nil
}

func runPush(cfg Config, args []string) error {
	fs := flag.NewFlagSet("push", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	envFile := fs.String("file", ".env", "Path to env file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	fileProvided := flagWasPassed(fs, "file")

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{
		AllowCreate: true,
	})
	if err != nil {
		return err
	}

	if fileProvided && *envFile == "" {
		return errors.New("source file cannot be empty")
	}
	if !fileProvided && canPrompt(cfg) {
		selected, err := promptText("Source file", *envFile, true)
		if err != nil {
			return err
		}
		*envFile = selected
	}

	keys, values, err := parseEnvFile(*envFile)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("no secrets found in %s", *envFile)
	}

	if canPrompt(cfg) {
		ok, err := promptConfirm(fmt.Sprintf("Found %d secrets. Push to project?", len(keys)), true)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Push canceled.")
			return nil
		}
	}

	for _, name := range keys {
		if err := upsertSecret(client, cfg, projectID, name, values[name]); err != nil {
			return fmt.Errorf("failed to push %q: %w", name, err)
		}
	}

	if cfg.CIToken == "" {
		message := fmt.Sprintf("Pushed %d secrets via CLI push (%s)", len(keys), *envFile)
		if err := createAuditEvent(client, cfg, projectID, "pushed", message); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write audit event: %v\n", err)
		}
	}

	successf("Pushed %d secret(s)", len(keys))
	return nil
}

func runSecretNames(cfg Config, args []string) error {
	fs := flag.NewFlagSet("secret-names", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	names, err := getSecretNames(client, cfg, projectID)
	if err != nil {
		return err
	}

	sort.Strings(names)
	for _, name := range names {
		fmt.Println(name)
	}
	return nil
}

func runSecretGet(cfg Config, args []string) error {
	fs := flag.NewFlagSet("secret-get", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	name := fs.String("name", "", "Secret name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("secret name", "pass --name")
		}
		secretName, err = promptText("Secret name", "", true)
		if err != nil {
			return err
		}
	}

	value, err := getSecret(client, cfg, projectID, secretName)
	if err != nil {
		return err
	}

	fmt.Printf("%s=%s\n", secretName, formatEnvValue(value))
	return nil
}

func runSecretSet(cfg Config, args []string) error {
	fs := flag.NewFlagSet("secret-set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	name := fs.String("name", "", "Secret name")
	value := fs.String("value", "", "Secret value")
	valueProvided := false
	for _, arg := range args {
		if arg == "--value" || strings.HasPrefix(arg, "--value=") {
			valueProvided = true
			break
		}
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("secret-set command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("secret name", "pass --name")
		}
		secretName, err = promptText("Secret name", "", true)
		if err != nil {
			return err
		}
	}
	var secretValue string
	if valueProvided {
		secretValue = *value
	} else {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("secret value", "pass --value")
		}
		secretValue, err = promptSecret("Value")
		if err != nil {
			return err
		}
	}

	if err := upsertSecret(client, cfg, projectID, secretName, secretValue); err != nil {
		return err
	}

	successf("Secret saved")
	return nil
}

func runSecretDelete(cfg Config, args []string) error {
	fs := flag.NewFlagSet("secret-delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	name := fs.String("name", "", "Secret name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("secret-delete command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("secret name", "pass --name")
		}
		secretName, err = promptText("Secret name", "", true)
		if err != nil {
			return err
		}
	}

	if canPrompt(cfg) {
		ok, err := promptConfirm(fmt.Sprintf("Delete %s? This cannot be undone.", secretName), false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Delete canceled.")
			return nil
		}
	}

	if err := deleteSecret(client, cfg, projectID, secretName); err != nil {
		return err
	}

	successf("Deleted")
	return nil
}

func runProjects(cfg Config, args []string) error {
	fs := flag.NewFlagSet("projects", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("projects command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projects, err := listProjects(client, cfg)
	if err != nil {
		return err
	}

	// userID, err := getCurrentUserID(client, cfg)
	// if err != nil {
	// 	return err
	// }

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return nil
	}

	sortProjects(projects)
	defaultProjectID, _, _ := resolveConfiguredProjectID("")

	for _, project := range projects {
		role := strings.TrimSpace(project.Role)
		if role == "" {
			role = "member"
		}
		ciTokenStatus := "no"
		if project.CITokenSet {
			ciTokenStatus = "yes"
		}
		marker := " "
		if strings.TrimSpace(project.ProjectID) == defaultProjectID {
			marker = "*"
		}
		fmt.Printf("%s %s\t%s\tci-token:%s\n", marker, projectDisplayName(project), role, ciTokenStatus)
	}
	return nil
}

func runProjectSet(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-set", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID := strings.TrimSpace(*projectIDFlag)
	projectLabel := ""
	if projectID == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("project id", "pass --project-id")
		}
		if cfg.CIToken != "" {
			return errors.New("project-set command requires user auth (ENVIS_CI_TOKEN is not supported)")
		}
		client, err := newHTTPClient(cfg)
		if err != nil {
			return err
		}
		project, err := promptProjectSelection(cfg, client, ProjectResolutionOptions{})
		if err != nil {
			return err
		}
		projectID = strings.TrimSpace(project.ProjectID)
		projectLabel = projectDisplayName(project)
	}

	if err := writeProjectID(cfg.ProjectPath, projectID); err != nil {
		return err
	}
	if projectLabel == "" {
		successf("Default project set")
	} else {
		successf("Default project set to %s", projectLabel)
	}
	return nil
}

func runProjectCreate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	name := fs.String("name", "", "Project name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectName := strings.TrimSpace(*name)
	if projectName == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("project name", "pass --name")
		}
		var err error
		projectName, err = promptText("Project name", "", true)
		if err != nil {
			return err
		}
	}

	if cfg.CIToken != "" {
		return errors.New("project-create command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	project, err := createProject(client, cfg, projectName)
	if err != nil {
		return err
	}

	successf("Project created: %s", project.Name)
	if canPrompt(cfg) {
		ok, err := promptConfirm("Set as default project?", true)
		if err != nil {
			return err
		}
		if ok {
			if err := writeProjectID(cfg.ProjectPath, project.ProjectID); err != nil {
				return err
			}
			successf("Default project set")
		}
	}
	return nil
}

func runProjectRename(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-rename", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	name := fs.String("name", "", "New project name")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-rename command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	newName := strings.TrimSpace(*name)
	if newName == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("project name", "pass --name")
		}
		defaultName := ""
		if project, err := getProjectByID(client, cfg, projectID); err == nil && project != nil {
			defaultName = strings.TrimSpace(project.Name)
		}
		newName, err = promptText("New name", defaultName, true)
		if err != nil {
			return err
		}
	}

	project, err := renameProject(client, cfg, projectID, newName)
	if err != nil {
		return err
	}

	successf("Renamed to %s", project.Name)
	return nil
}

func runProjectDelete(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-delete command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	if canPrompt(cfg) {
		projectName := projectID
		if project, err := getProjectByID(client, cfg, projectID); err == nil && project != nil && strings.TrimSpace(project.Name) != "" {
			projectName = strings.TrimSpace(project.Name)
		}
		warnf("This will delete all secrets in this project.")
		typed, err := promptText("Type the project name to confirm", "", true)
		if err != nil {
			return err
		}
		if typed != projectName {
			fmt.Println("Delete canceled.")
			return nil
		}
	}

	if err := deleteProject(client, cfg, projectID); err != nil {
		return err
	}

	successf("Deleted")
	return nil
}

func runProjectMembers(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-members", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-members command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	resp, err := listProjectMembers(client, cfg, projectID)
	if err != nil {
		return err
	}

	if len(resp.Members) == 0 {
		fmt.Println("No members found.")
		return nil
	}

	for _, member := range resp.Members {
		role := strings.TrimSpace(member.Role)
		if role == "" {
			role = "member"
		}
		fmt.Printf("%s\t%s\n", memberDisplayName(member), role)
	}
	return nil
}

func runProjectMemberRemove(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-member-remove", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	userIDFlag := fs.String("user-id", "", "User UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-member-remove command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	userID := strings.TrimSpace(*userIDFlag)
	memberLabel := userID
	if userID == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("user id", "pass --user-id")
		}
		resp, err := listProjectMembers(client, cfg, projectID)
		if err != nil {
			return err
		}
		var members []ProjectMember
		var options []string
		for _, member := range resp.Members {
			if strings.TrimSpace(member.ID) == "" {
				continue
			}
			members = append(members, member)
			options = append(options, memberDisplayName(member))
		}
		index, err := promptSelect("Select member to remove", options)
		if err != nil {
			return err
		}
		userID = strings.TrimSpace(members[index].ID)
		memberLabel = memberDisplayName(members[index])
	}

	if canPrompt(cfg) {
		ok, err := promptConfirm(fmt.Sprintf("Remove %s?", memberLabel), false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Remove canceled.")
			return nil
		}
	}

	if err := removeProjectMember(client, cfg, projectID, userID); err != nil {
		return err
	}

	successf("Removed")
	return nil
}

func runCiTokenGenerate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("ci-token-generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-generate command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	if canPrompt(cfg) {
		warnf("Store this token securely. It will not be shown again.")
	}
	token, err := generateCIToken(client, cfg, projectID)
	if err != nil {
		return err
	}

	successf("Token: %s", token)
	return nil
}

func runCiTokenReset(cfg Config, args []string) error {
	fs := flag.NewFlagSet("ci-token-reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-reset command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	if canPrompt(cfg) {
		warnf("This will invalidate the existing CI token immediately.")
		ok, err := promptConfirm("Confirm reset?", false)
		if err != nil {
			return err
		}
		if !ok {
			fmt.Println("Reset canceled.")
			return nil
		}
	}

	token, err := generateCIToken(client, cfg, projectID)
	if err != nil {
		return err
	}

	successf("New token: %s", token)
	return nil
}

func runCiTokenVerify(cfg Config, args []string) error {
	fs := flag.NewFlagSet("ci-token-verify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	token := fs.String("token", "", "CI token to verify")
	tokenProvided := false
	for _, arg := range args {
		if arg == "--token" || strings.HasPrefix(arg, "--token=") {
			tokenProvided = true
			break
		}
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-verify command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	tokenValue := strings.TrimSpace(*token)
	if !tokenProvided || tokenValue == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("token", "pass --token")
		}
		tokenValue, err = promptSecret("Token")
		if err != nil {
			return err
		}
	}

	ok, err := verifyCIToken(client, cfg, projectID, tokenValue)
	if err != nil {
		return err
	}

	if ok {
		successf("Valid")
	} else {
		fmt.Println("✗ Invalid")
	}
	return nil
}

func runInvites(cfg Config, args []string) error {
	fs := flag.NewFlagSet("invites", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("invites command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	invites, err := listMyInvites(client, cfg)
	if err != nil {
		return err
	}

	if len(invites) == 0 {
		fmt.Println("No pending invites.")
		return nil
	}

	for _, invite := range invites {
		fmt.Printf("%s\tfrom %s\n", inviteProjectDisplayName(invite), inviteFromDisplayName(invite))
	}

	return nil
}

func runInviteRespond(cfg Config, args []string) error {
	fs := flag.NewFlagSet("invite-respond", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	inviteIDFlag := fs.String("invite-id", "", "Invite UUID")
	accept := fs.Bool("accept", false, "Accept the invite")
	reject := fs.Bool("reject", false, "Reject the invite")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("invite-respond command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	inviteID := strings.TrimSpace(*inviteIDFlag)
	if inviteID == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("invite id", "pass --invite-id")
		}
		invites, err := listMyInvites(client, cfg)
		if err != nil {
			return err
		}
		options := make([]string, 0, len(invites))
		for _, invite := range invites {
			options = append(options, inviteDisplayName(invite))
		}
		index, err := promptSelect("Select invite", options)
		if err != nil {
			return err
		}
		inviteID = strings.TrimSpace(invites[index].ID)
	}

	action := "accept"
	if *accept == *reject {
		if !canPrompt(cfg) {
			return errors.New("choose exactly one of --accept or --reject")
		}
		index, err := promptSelect("Accept or reject?", []string{"Accept", "Reject"})
		if err != nil {
			return err
		}
		if index == 1 {
			action = "reject"
		}
	} else if *reject {
		action = "reject"
	}

	updated, err := respondToInvite(client, cfg, inviteID, action)
	if err != nil {
		return err
	}

	if action == "accept" {
		successf("Joined %s", strings.TrimSpace(updated.ProjectName))
	} else {
		successf("Invite rejected")
	}
	return nil
}

func runInviteCreate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("invite-create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	email := fs.String("email", "", "Recipient email")
	role := fs.String("role", "member", "Invite role: admin|member")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("invite-create command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	projectID, err := resolveProjectIDInteractive(cfg, client, *projectIDFlag, ProjectResolutionOptions{})
	if err != nil {
		return err
	}

	recipient := strings.TrimSpace(*email)
	if recipient == "" {
		if !canPrompt(cfg) {
			return nonInteractiveInputError("email", "pass --email")
		}
		recipient, err = promptText("Email", "", true)
		if err != nil {
			return err
		}
	}
	inviteRole := strings.ToLower(strings.TrimSpace(*role))
	if inviteRole == "" {
		inviteRole = "member"
	}
	if inviteRole != "admin" && inviteRole != "member" {
		return errors.New("invalid role: must be admin or member")
	}

	invite, err := createInvite(client, cfg, projectID, recipient, inviteRole)
	if err != nil {
		return err
	}

	_ = invite
	successf("Invite sent to %s", recipient)
	return nil
}

func runStatus(cfg Config, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	fmt.Printf("Session file: %s\n", cfg.SessionPath)

	if cfg.CIToken != "" {
		fmt.Println("Auth mode: ci-token")
		fmt.Println("Signed in: yes (ci-token)")
		printConfiguredProjectStatus(nil, cfg)
		return nil
	}

	session, err := loadSession(cfg.SessionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Auth mode: user")
			fmt.Println("Signed in: no")
			printConfiguredProjectStatus(nil, cfg)
			return nil
		}
		return err
	}

	if session.AccessToken == "" && session.RefreshToken == "" {
		fmt.Println("Auth mode: user")
		fmt.Println("Signed in: no (invalid session)")
		printConfiguredProjectStatus(nil, cfg)
		return nil
	}

	if session.AccessToken == "" || shouldRefresh(session) {
		refreshed, err := refreshSession(cfg, session)
		if err != nil {
			return fmt.Errorf("session expired: %w", err)
		}
		session = refreshed
	}

	headers := map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + session.AccessToken,
	}
	if userID := sessionUserID(session); userID != "" && isLocalURL(cfg.BaseURL) {
		headers["X-User-Id"] = userID
	}

	client := &http.Client{
		Transport: &headerRoundTripper{
			next:    http.DefaultTransport,
			headers: headers,
		},
		Timeout: 10 * time.Second,
	}

	profile, err := getCurrentUserProfile(client, cfg)
	if err != nil {
		fmt.Println("Auth mode: user")
		fmt.Println("Signed in: yes")
		fmt.Printf("User info unavailable: %v\n", err)
		printConfiguredProjectStatus(client, cfg)
		return nil
	}

	fmt.Println("Auth mode: user")
	fmt.Println("Signed in: yes")
	if strings.TrimSpace(profile.Email) != "" {
		fmt.Printf("Email: %s\n", strings.TrimSpace(profile.Email))
	}
	if strings.TrimSpace(profile.Name) != "" {
		fmt.Printf("Name: %s\n", strings.TrimSpace(profile.Name))
	}
	printConfiguredProjectStatus(client, cfg)
	return nil
}

func runUpdate(args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	installPath, err := resolveInstallPath()
	if err != nil {
		return err
	}

	osName, arch, err := normalizeOSArch()
	if err != nil {
		return err
	}

	assetURL, err := latestReleaseAssetURL(defaultRepo, osName, arch)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "envis-update-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	tarPath := filepath.Join(tmpDir, cliBinaryName+".tar.gz")
	if err := downloadFile(assetURL, tarPath); err != nil {
		return err
	}

	extractedPath := filepath.Join(tmpDir, cliBinaryName)
	if err := extractTarGzBinary(tarPath, cliBinaryName, extractedPath); err != nil {
		return err
	}

	if err := installBinary(extractedPath, installPath); err != nil {
		return err
	}

	fmt.Printf("Updated %s at %s\n", cliBinaryName, installPath)
	return nil
}

func runUninstall(args []string) error {
	fs := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(args); err != nil {
		return err
	}

	installPath, err := resolveInstallPath()
	if err != nil {
		return err
	}

	if err := os.Remove(installPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%s not found at %s", cliBinaryName, installPath)
		}
		return fmt.Errorf("failed to remove %s: %w", installPath, err)
	}

	fmt.Printf("Uninstalled %s from %s\n", cliBinaryName, installPath)
	return nil
}

func canPrompt(cfg Config) bool {
	if cfg.CIToken != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stderr.Fd()))
}

func nonInteractiveInputError(name, hint string) error {
	if hint == "" {
		return fmt.Errorf("missing %s in non-interactive mode", name)
	}
	return fmt.Errorf("missing %s in non-interactive mode: %s", name, hint)
}

func promptText(label, defaultValue string, required bool) (string, error) {
	for {
		if defaultValue != "" {
			fmt.Fprintf(os.Stderr, "? %s: (%s) ", label, defaultValue)
		} else {
			fmt.Fprintf(os.Stderr, "? %s: ", label)
		}

		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		value := strings.TrimSpace(line)
		if value == "" {
			value = defaultValue
		}
		if value != "" || !required {
			return value, nil
		}
		fmt.Fprintln(os.Stderr, "Value is required.")
	}
}

func promptSecret(label string) (string, error) {
	for {
		fmt.Fprintf(os.Stderr, "? %s: ", label)
		value, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		if len(value) > 0 {
			return string(value), nil
		}
		fmt.Fprintln(os.Stderr, "Value is required.")
	}
}

func promptConfirm(label string, defaultYes bool) (bool, error) {
	suffix := "(y/N)"
	if defaultYes {
		suffix = "(Y/n)"
	}
	for {
		fmt.Fprintf(os.Stderr, "? %s %s ", label, suffix)
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return false, err
		}
		answer := strings.ToLower(strings.TrimSpace(line))
		if answer == "" {
			return defaultYes, nil
		}
		switch answer {
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Fprintln(os.Stderr, "Enter y or n.")
		}
	}
}

func promptSelect(label string, options []string) (int, error) {
	if len(options) == 0 {
		return -1, errors.New("no options available")
	}
	if len(options) == 1 {
		return 0, nil
	}

	fmt.Fprintf(os.Stderr, "? %s:\n", label)
	for i, option := range options {
		fmt.Fprintf(os.Stderr, "  %d. %s\n", i+1, option)
	}
	for {
		fmt.Fprintf(os.Stderr, "Select 1-%d: ", len(options))
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return -1, err
		}
		value, err := strconv.Atoi(strings.TrimSpace(line))
		if err == nil && value >= 1 && value <= len(options) {
			return value - 1, nil
		}
		fmt.Fprintln(os.Stderr, "Enter a valid number.")
	}
}

func flagWasPassed(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

func successf(format string, args ...interface{}) {
	fmt.Printf("✓ "+format+"\n", args...)
}

func warnf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "⚠ "+format+"\n", args...)
}

func printConfiguredProjectStatus(client *http.Client, cfg Config) {
	projectID, ok, err := resolveConfiguredProjectID("")
	if err != nil {
		fmt.Printf("Current project: unavailable (%v)\n", err)
		return
	}
	if !ok {
		fmt.Println("Current project: not set")
		return
	}
	if client == nil {
		if strings.TrimSpace(os.Getenv("ENVIS_PROJECT_ID")) != "" {
			fmt.Println("Current project: set via ENVIS_PROJECT_ID")
			return
		}
		fmt.Println("Current project: set")
		return
	}
	project, err := getProjectByID(client, cfg, projectID)
	if err != nil {
		fmt.Printf("Current project: set (details unavailable: %v)\n", err)
		return
	}
	if project == nil {
		fmt.Println("Current project: set (not found in your projects)")
		return
	}
	fmt.Printf("Current project: %s\n", projectDisplayName(*project))
}

func resolveConfiguredProjectID(flagValue string) (string, bool, error) {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue), true, nil
	}

	if value := strings.TrimSpace(os.Getenv("ENVIS_PROJECT_ID")); value != "" {
		return value, true, nil
	}

	if value, err := readProjectID(); err == nil && value != "" {
		return value, true, nil
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", false, err
	}

	return "", false, nil
}

func resolveProjectIDInteractive(cfg Config, client *http.Client, flagValue string, opts ProjectResolutionOptions) (string, error) {
	if projectID, ok, err := resolveConfiguredProjectID(flagValue); err != nil {
		return "", err
	} else if ok {
		return projectID, nil
	}

	if !canPrompt(cfg) {
		return "", nonInteractiveInputError("project id", "pass --project-id, set ENVIS_PROJECT_ID, or run `envis project-set`")
	}

	fmt.Fprintln(os.Stderr, "No default project set.")
	project, err := promptProjectSelection(cfg, client, opts)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(project.ProjectID), nil
}

func promptProjectSelection(cfg Config, client *http.Client, opts ProjectResolutionOptions) (Project, error) {
	label := strings.TrimSpace(opts.Prompt)
	if label == "" {
		label = "Select a project"
	}

	projects, err := listProjects(client, cfg)
	if err != nil {
		return Project{}, err
	}
	sortProjects(projects)

	if len(projects) == 1 {
		return projects[0], nil
	}

	options := make([]string, 0, len(projects)+2)
	for _, project := range projects {
		options = append(options, projectDisplayName(project))
	}
	createIndex := -1
	manualIndex := -1
	if opts.AllowCreate {
		createIndex = len(options)
		options = append(options, "+ Create new project")
	}
	if opts.AllowManual {
		manualIndex = len(options)
		options = append(options, "+ Set manually")
	}

	if len(options) == 0 {
		if !opts.AllowCreate && !opts.AllowManual {
			return Project{}, errors.New("no projects found")
		}
		if opts.AllowCreate {
			createIndex = len(options)
			options = append(options, "+ Create new project")
		}
		if opts.AllowManual {
			manualIndex = len(options)
			options = append(options, "+ Set manually")
		}
	}

	index, err := promptSelect(label, options)
	if err != nil {
		return Project{}, err
	}
	switch index {
	case createIndex:
		name, err := promptText("Project name", "", true)
		if err != nil {
			return Project{}, err
		}
		project, err := createProject(client, cfg, name)
		if err != nil {
			return Project{}, err
		}
		successf("Project created: %s", project.Name)
		return project, nil
	case manualIndex:
		projectID, err := promptText("Project ID", "", true)
		if err != nil {
			return Project{}, err
		}
		return Project{ProjectID: projectID, Name: projectID}, nil
	default:
		if index < 0 || index >= len(projects) {
			return Project{}, errors.New("invalid project selection")
		}
		return projects[index], nil
	}
}

func sortProjects(projects []Project) {
	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})
}

func projectDisplayName(project Project) string {
	name := strings.TrimSpace(project.Name)
	if name == "" {
		return "Unnamed project"
	}
	return name
}

func memberDisplayName(member ProjectMember) string {
	email := strings.TrimSpace(member.Email)
	if email != "" {
		return email
	}
	name := strings.TrimSpace(member.Name)
	if name != "" {
		return name
	}
	return "Unknown member"
}

func inviteDisplayName(invite Invite) string {
	project := inviteProjectDisplayName(invite)
	from := inviteFromDisplayName(invite)
	if from == "" {
		return project
	}
	return fmt.Sprintf("%s (from %s)", project, from)
}

func inviteProjectDisplayName(invite Invite) string {
	project := strings.TrimSpace(invite.ProjectName)
	if project == "" {
		return "Unknown project"
	}
	return project
}

func inviteFromDisplayName(invite Invite) string {
	from := strings.TrimSpace(invite.InviterEmail)
	if from == "" {
		from = strings.TrimSpace(invite.InviterName)
	}
	if from == "" {
		return "unknown sender"
	}
	return from
}

func resolveProjectID(flagValue string) (string, error) {
	if projectID, ok, err := resolveConfiguredProjectID(flagValue); err != nil {
		return "", err
	} else if ok {
		return projectID, nil
	}

	return "", errors.New("missing project id: pass --project-id, set ENVIS_PROJECT_ID, or run `envis project-set`")
}

func newConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to resolve home dir: %w", err)
	}

	localURI := strings.TrimSpace(os.Getenv("LOCAL_URI"))
	baseURL := strings.TrimRight(defaultAPIURL, "/")
	if localURI != "" {
		baseURL = strings.TrimRight(localURI, "/")
	}
	dashURL := strings.TrimRight(defaultDashboardURL, "/")
	if dashboardURI := strings.TrimSpace(os.Getenv("DASHBOARD_URI")); dashboardURI != "" {
		dashURL = strings.TrimRight(dashboardURI, "/")
	}
	apiVersion := strings.ToLower(strings.TrimSpace(os.Getenv("version")))
	if apiVersion == "" {
		apiVersion = strings.ToLower(strings.TrimSpace(os.Getenv("VERSION")))
	}
	if apiVersion == "" {
		apiVersion = "v1"
	}
	if apiVersion != "v1" && apiVersion != "v2" {
		return Config{}, fmt.Errorf("invalid version %q: expected v1 or v2", apiVersion)
	}

	return Config{
		BaseURL:      baseURL,
		DashboardURL: dashURL,
		APIVersion:   apiVersion,
		SessionPath:  filepath.Join(home, ".envis", "session.json"),
		ProjectPath:  filepath.Join(home, ".envis", "project_id"),
		CIToken:      strings.TrimSpace(os.Getenv("ENVIS_CI_TOKEN")),
	}, nil
}

func authEndpoint(cfg Config, suffix string) string {
	return cfg.BaseURL + "/" + cfg.APIVersion + "/auth" + suffix
}

func currentUserEndpoint(cfg Config) string {
	if cfg.APIVersion == "v2" {
		return authEndpoint(cfg, "/me")
	}
	return cfg.BaseURL + "/" + cfg.APIVersion + "/me"
}

func apiEndpoint(cfg Config, path string) string {
	return cfg.BaseURL + "/" + cfg.APIVersion + path
}

func newHTTPClient(cfg Config) (*http.Client, error) {
	headers, err := authHeaders(cfg)
	if err != nil {
		return nil, err
	}

	transport := &headerRoundTripper{
		next:    http.DefaultTransport,
		headers: headers,
	}
	return &http.Client{Transport: transport, Timeout: 15 * time.Second}, nil
}

func authHeaders(cfg Config) (map[string]string, error) {
	headers := map[string]string{"Content-Type": "application/json"}
	if cfg.CIToken != "" {
		headers["X-CI-Token"] = cfg.CIToken
		return headers, nil
	}

	session, err := ensureSession(cfg)
	if err != nil {
		return nil, err
	}

	headers["Authorization"] = "Bearer " + session.AccessToken
	if userID := sessionUserID(session); userID != "" && isLocalURL(cfg.BaseURL) {
		headers["X-User-Id"] = userID
	}
	return headers, nil
}

func ensureSession(cfg Config) (Session, error) {
	session, err := loadSession(cfg.SessionPath)
	if err == nil {
		if shouldRefresh(session) {
			return refreshSession(cfg, session)
		}
		if session.AccessToken == "" && session.RefreshToken != "" {
			return refreshSession(cfg, session)
		}
		if session.AccessToken == "" {
			_ = os.Remove(cfg.SessionPath)
			return Session{}, errors.New("session is invalid; run `envis login`")
		}
		return session, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return Session{}, err
	}

	if cfg.CIToken != "" {
		return Session{}, errors.New("no cached session and ENVIS_CI_TOKEN flow does not require login")
	}

	session, err = performDeviceLogin(cfg)
	if err != nil {
		return Session{}, err
	}
	return session, nil
}

func loadSession(path string) (Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Session{}, err
	}

	var session Session
	if err := json.Unmarshal(b, &session); err != nil {
		return Session{}, fmt.Errorf("session file is corrupt: %w", err)
	}
	return session, nil
}

func readProjectID() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}
	path := filepath.Join(home, ".envis", "project_id")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

func writeProjectID(path, projectID string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}
	if err := os.WriteFile(path, []byte(strings.TrimSpace(projectID)+"\n"), 0o600); err != nil {
		return fmt.Errorf("failed to write project id: %w", err)
	}
	return nil
}

func writeSession(path string, session Session) error {
	if session.AccessToken == "" || session.RefreshToken == "" {
		return errors.New("session payload missing required fields")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("failed to create session dir: %w", err)
	}

	payload, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	if err := os.WriteFile(path, payload, 0o600); err != nil {
		return fmt.Errorf("failed to write session: %w", err)
	}

	return nil
}

func shouldRefresh(session Session) bool {
	if session.ExpiresAt == 0 {
		return false
	}
	return time.Now().Unix() >= (session.ExpiresAt - 60)
}

func refreshSession(cfg Config, session Session) (Session, error) {
	if session.RefreshToken == "" {
		return Session{}, errors.New("session expired and no refresh token is available; run `envis login`")
	}

	endpoint := authEndpoint(cfg, "/refresh")
	body := map[string]string{"refresh_token": session.RefreshToken}
	bodyJSON, _ := json.Marshal(body)

	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyJSON))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("failed to reach Envault API: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Session{}, fmt.Errorf("failed to refresh session (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var refreshed Session
	if err := json.Unmarshal(respBody, &refreshed); err != nil {
		return Session{}, fmt.Errorf("refresh endpoint returned invalid JSON: %w", err)
	}

	if refreshed.AccessToken == "" || refreshed.RefreshToken == "" {
		return Session{}, errors.New("refresh endpoint returned incomplete session")
	}

	if err := writeSession(cfg.SessionPath, refreshed); err != nil {
		return Session{}, err
	}

	return refreshed, nil
}

func performDeviceLogin(cfg Config) (Session, error) {
	deviceCode, err := newUUIDv4()
	if err != nil {
		return Session{}, err
	}

	authURL := cfg.DashboardURL + "/auth?device_code=" + url.QueryEscape(deviceCode)
	fmt.Printf("No cached session found. Open this URL to authenticate:\n%s\n", authURL)
	if err := openBrowser(authURL); err != nil {
		fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
	}
	fmt.Println("Waiting for approval...")

	endpoint := authEndpoint(cfg, "/"+url.PathEscape(deviceCode))
	deadline := time.Now().Add(pollWaitTimeout)

	for time.Now().Before(deadline) {
		req, err := http.NewRequest(http.MethodGet, endpoint, nil)
		if err != nil {
			return Session{}, err
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(defaultPollDelay)
			continue
		}

		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		var payload PollAuthResponse
		if err := json.Unmarshal(body, &payload); err != nil {
			if resp.StatusCode == http.StatusAccepted {
				time.Sleep(retryAfterOrDefault(resp.Header.Get("Retry-After"), defaultPollDelay))
				continue
			}
			preview := strings.TrimSpace(string(body))
			if preview == "" {
				preview = "<empty body>"
			}
			if len(preview) > 200 {
				preview = preview[:200]
			}
			return Session{}, fmt.Errorf("auth endpoint returned invalid JSON (status %d): %s", resp.StatusCode, preview)
		}

		if resp.StatusCode == http.StatusOK && payload.IsAuth {
			var session Session
			if err := json.Unmarshal(payload.Content, &session); err != nil {
				return Session{}, fmt.Errorf("invalid session payload from auth endpoint: %w", err)
			}

			if err := writeSession(cfg.SessionPath, session); err != nil {
				return Session{}, err
			}
			return session, nil
		}

		if resp.StatusCode == http.StatusAccepted {
			time.Sleep(retryAfterOrDefault(resp.Header.Get("Retry-After"), defaultPollDelay))
			continue
		}

		detail := strings.TrimSpace(payload.Detail)
		if detail == "" {
			detail = strings.TrimSpace(string(body))
		}
		return Session{}, fmt.Errorf("auth endpoint failed (%d): %s", resp.StatusCode, detail)
	}

	return Session{}, errors.New("authentication timed out")
}

func listSecretNames(client *http.Client, cfg Config, projectID string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var payload SecretListResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("invalid list response: %w", err)
	}

	return payload.Secrets, nil
}

func getSecretNames(client *http.Client, cfg Config, projectID string) ([]string, error) {
	return listSecretNames(client, cfg, projectID)
}

func listProjects(client *http.Client, cfg Config) ([]Project, error) {
	endpoint := apiEndpoint(cfg, "/projects")
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var projects []Project
	if err := json.Unmarshal(respBody, &projects); err != nil {
		return nil, fmt.Errorf("invalid projects response: %w", err)
	}
	return projects, nil
}

func getProjectByID(client *http.Client, cfg Config, projectID string) (*Project, error) {
	projects, err := listProjects(client, cfg)
	if err != nil {
		return nil, err
	}

	for _, project := range projects {
		if strings.TrimSpace(project.ProjectID) == projectID {
			return &project, nil
		}
	}

	return nil, nil
}

func getCurrentUserID(client *http.Client, cfg Config) (string, error) {
	endpoint := currentUserEndpoint(cfg)
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	var user CurrentUser
	if err := json.Unmarshal(respBody, &user); err != nil {
		return "", fmt.Errorf("invalid /%s/me response: %w", cfg.APIVersion, err)
	}
	user.ID = strings.TrimSpace(user.ID)
	if user.ID == "" {
		return "", errors.New("unable to determine authenticated user id")
	}
	return user.ID, nil
}

func getCurrentUserProfile(client *http.Client, cfg Config) (CurrentUserProfile, error) {
	endpoint := currentUserEndpoint(cfg)
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return CurrentUserProfile{}, err
	}

	var user CurrentUserProfile
	if err := json.Unmarshal(respBody, &user); err != nil {
		return CurrentUserProfile{}, fmt.Errorf("invalid /%s/me response: %w", cfg.APIVersion, err)
	}
	return user, nil
}

func getSecret(client *http.Client, cfg Config, projectID string, name string) (string, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID), url.PathEscape(name))
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	var payload SecretGetResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("invalid secret response for %q: %w", name, err)
	}

	return payload.Value, nil
}

func getAllSecrets(client *http.Client, cfg Config, projectID string) ([]SecretValue, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets/all", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var payload SecretValuesResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return nil, fmt.Errorf("invalid secret list response: %w", err)
	}
	return payload.Secrets, nil
}

func getBatchSecrets(client *http.Client, cfg Config, projectID string, names []string) ([]SecretValue, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets/batch", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	payload := SecretsBatchRequest{Names: names}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to encode payload: %w", err)
	}

	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, body)
	if err != nil {
		return nil, err
	}

	var resp SecretValuesResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("invalid batch response: %w", err)
	}
	return resp.Secrets, nil
}

func upsertSecret(client *http.Client, cfg Config, projectID string, name string, value string) error {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID), url.PathEscape(name))
	payload := SecretUpsertRequest{Value: value}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	if _, err := doJSONRequest(client, http.MethodPut, endpoint, body); err != nil {
		return err
	}
	return nil
}

func deleteSecret(client *http.Client, cfg Config, projectID, name string) error {
	endpoint := fmt.Sprintf("%s/projects/%s/secrets/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID), url.PathEscape(name))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func createAuditEvent(client *http.Client, cfg Config, projectID, action, message string) error {
	endpoint := fmt.Sprintf("%s/projects/%s/audit", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	payload := AuditCreateRequest{
		Action:  strings.TrimSpace(action),
		Message: strings.TrimSpace(message),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode payload: %w", err)
	}

	if _, err := doJSONRequest(client, http.MethodPost, endpoint, body); err != nil {
		return err
	}
	return nil
}

func listProjectMembers(client *http.Client, cfg Config, projectID string) (ProjectMembersResponse, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/members", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return ProjectMembersResponse{}, err
	}

	var payload ProjectMembersResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return ProjectMembersResponse{}, fmt.Errorf("invalid members response: %w", err)
	}
	return payload, nil
}

func removeProjectMember(client *http.Client, cfg Config, projectID, userID string) error {
	endpoint := fmt.Sprintf("%s/projects/%s/members/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID), url.PathEscape(userID))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func listMyInvites(client *http.Client, cfg Config) ([]Invite, error) {
	endpoint := apiEndpoint(cfg, "/me/invites")
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	var invites []Invite
	if err := json.Unmarshal(respBody, &invites); err != nil {
		return nil, fmt.Errorf("invalid invites response: %w", err)
	}
	return invites, nil
}

func respondToInvite(client *http.Client, cfg Config, inviteID string, action string) (Invite, error) {
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "accept" && action != "reject" {
		return Invite{}, errors.New("action must be accept or reject")
	}

	endpoint := fmt.Sprintf("%s/invites/%s/%s", apiEndpoint(cfg, ""), url.PathEscape(inviteID), action)
	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, nil)
	if err != nil {
		return Invite{}, err
	}

	var invite Invite
	if err := json.Unmarshal(respBody, &invite); err != nil {
		return Invite{}, fmt.Errorf("invalid invite response: %w", err)
	}
	return invite, nil
}

func createInvite(client *http.Client, cfg Config, projectID, email, role string) (Invite, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/invite", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	payload := InviteCreateRequest{Email: strings.TrimSpace(email), Role: role}
	body, err := json.Marshal(payload)
	if err != nil {
		return Invite{}, fmt.Errorf("failed to encode payload: %w", err)
	}

	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, body)
	if err != nil {
		return Invite{}, err
	}

	var invite Invite
	if err := json.Unmarshal(respBody, &invite); err != nil {
		return Invite{}, fmt.Errorf("invalid invite response: %w", err)
	}
	return invite, nil
}

func createProject(client *http.Client, cfg Config, name string) (Project, error) {
	endpoint := apiEndpoint(cfg, "/projects")
	payload := ProjectCreateRequest{Name: strings.TrimSpace(name)}
	body, err := json.Marshal(payload)
	if err != nil {
		return Project{}, fmt.Errorf("failed to encode payload: %w", err)
	}

	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, body)
	if err != nil {
		return Project{}, err
	}

	var project Project
	if err := json.Unmarshal(respBody, &project); err != nil {
		return Project{}, fmt.Errorf("invalid project response: %w", err)
	}
	return project, nil
}

func renameProject(client *http.Client, cfg Config, projectID, newName string) (Project, error) {
	endpoint := fmt.Sprintf("%s/projects/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	payload := ProjectRenameRequest{NewName: strings.TrimSpace(newName)}
	body, err := json.Marshal(payload)
	if err != nil {
		return Project{}, fmt.Errorf("failed to encode payload: %w", err)
	}

	respBody, err := doJSONRequest(client, http.MethodPatch, endpoint, body)
	if err != nil {
		return Project{}, err
	}

	var project Project
	if err := json.Unmarshal(respBody, &project); err != nil {
		return Project{}, fmt.Errorf("invalid project response: %w", err)
	}
	return project, nil
}

func deleteProject(client *http.Client, cfg Config, projectID string) error {
	endpoint := fmt.Sprintf("%s/projects/%s", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func generateCIToken(client *http.Client, cfg Config, projectID string) (string, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/ci-token", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, nil)
	if err != nil {
		return "", err
	}

	var payload CiTokenResponse
	if err := json.Unmarshal(respBody, &payload); err != nil {
		return "", fmt.Errorf("invalid ci token response: %w", err)
	}
	token := strings.TrimSpace(payload.Token)
	if token == "" {
		return "", errors.New("ci token response missing token")
	}
	return token, nil
}

func verifyCIToken(client *http.Client, cfg Config, projectID, token string) (bool, error) {
	endpoint := fmt.Sprintf("%s/projects/%s/ci-token/verify", apiEndpoint(cfg, ""), url.PathEscape(projectID))
	payload := CiTokenVerifyRequest{Token: strings.TrimSpace(token)}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, fmt.Errorf("failed to encode payload: %w", err)
	}

	respBody, err := doJSONRequest(client, http.MethodPost, endpoint, body)
	if err != nil {
		return false, err
	}

	var resp CiTokenVerifyResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return false, fmt.Errorf("invalid ci token verify response: %w", err)
	}
	return resp.Status, nil
}

func doJSONRequest(client *http.Client, method, endpoint string, body []byte) ([]byte, error) {
	var reader io.Reader
	if len(body) > 0 {
		reader = bytes.NewReader(body)
	}

	req, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return nil, err
	}

	stopSpinner := startSpinner("Contacting Envisible")
	defer stopSpinner()

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api error (%d): %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	return respBody, nil
}

func startSpinner(label string) func() {
	if !term.IsTerminal(int(os.Stderr.Fd())) {
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		frames := []string{"-", "\\", "|", "/"}
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-done:
				fmt.Fprint(os.Stderr, "\r\033[K")
				return
			case <-ticker.C:
				fmt.Fprintf(os.Stderr, "\r%s %s", frames[i%len(frames)], label)
				i++
			}
		}
	}()

	return func() {
		close(done)
	}
}

type headerRoundTripper struct {
	next    http.RoundTripper
	headers map[string]string
}

func (t *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for key, value := range t.headers {
		clone.Header.Set(key, value)
	}
	return t.next.RoundTrip(clone)
}

func sessionUserID(session Session) string {
	if session.User == nil {
		return ""
	}
	v, ok := session.User["id"]
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func isLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "0.0.0.0"
}

func resolveInstallPath() (string, error) {
	if dir := strings.TrimSpace(os.Getenv("ENVIS_INSTALL_DIR")); dir != "" {
		return filepath.Join(dir, cliBinaryName), nil
	}

	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		if filepath.Base(exe) == cliBinaryName {
			return exe, nil
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "bin", cliBinaryName), nil
}

func normalizeOSArch() (string, string, error) {
	var osName string
	switch runtime.GOOS {
	case "linux", "darwin":
		osName = runtime.GOOS
	default:
		return "", "", fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}

	var arch string
	switch runtime.GOARCH {
	case "amd64", "x86_64":
		arch = "amd64"
	case "arm64":
		arch = "arm64"
	default:
		return "", "", fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
	}

	return osName, arch, nil
}

type ghRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func latestReleaseAssetURL(repo, osName, arch string) (string, error) {
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(endpoint)
	if err != nil {
		return "", fmt.Errorf("failed to fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("failed to fetch latest release (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var release ghRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("invalid release response: %w", err)
	}

	suffix := fmt.Sprintf("_%s_%s.tar.gz", osName, arch)
	for _, asset := range release.Assets {
		if strings.HasPrefix(asset.Name, cliBinaryName+"_") && strings.HasSuffix(asset.Name, suffix) {
			if strings.TrimSpace(asset.BrowserDownloadURL) != "" {
				return asset.BrowserDownloadURL, nil
			}
		}
	}

	return "", fmt.Errorf("no release asset found for %s/%s", osName, arch)
}

func downloadFile(url, path string) error {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("download failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}

	return nil
}

func extractTarGzBinary(archivePath, binName, outputPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", archivePath, err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("failed to read gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read archive: %w", err)
		}
		if hdr.FileInfo().IsDir() {
			continue
		}
		if filepath.Base(hdr.Name) != binName {
			continue
		}

		out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return fmt.Errorf("failed to write %s: %w", outputPath, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return fmt.Errorf("failed to write %s: %w", outputPath, err)
		}
		if err := out.Close(); err != nil {
			return fmt.Errorf("failed to write %s: %w", outputPath, err)
		}
		return nil
	}

	return fmt.Errorf("binary %s not found in archive", binName)
}

func installBinary(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("failed to create install dir: %w", err)
	}

	tmpPath := dst + ".tmp"
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("failed to write %s: %w", tmpPath, err)
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("failed to write %s: %w", tmpPath, err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("failed to write %s: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, dst); err != nil {
		return fmt.Errorf("failed to install %s: %w", dst, err)
	}

	return nil
}

func retryAfterOrDefault(header string, fallback time.Duration) time.Duration {
	if header == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(header))
	if err != nil || n <= 0 {
		return fallback
	}
	return time.Duration(n) * time.Second
}

func formatEnvValue(v string) string {
	if v == "" {
		return `""`
	}

	if isSimpleEnvValue(v) {
		return v
	}

	return strconv.Quote(v)
}

func isSimpleEnvValue(v string) bool {
	for _, r := range v {
		if (r >= 'a' && r <= 'z') ||
			(r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') ||
			r == '_' || r == '-' || r == '.' || r == ':' || r == '/' {
			continue
		}
		return false
	}
	return true
}

func openBrowser(target string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "linux":
		cmd = exec.Command("xdg-open", target)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", target)
	default:
		return fmt.Errorf("unsupported OS: %s", runtime.GOOS)
	}
	return cmd.Start()
}

func newUUIDv4() (string, error) {
	b := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return "", fmt.Errorf("failed to read random bytes: %w", err)
	}

	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80

	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4],
		b[4:6],
		b[6:8],
		b[8:10],
		b[10:16],
	), nil
}

func loadDotenvFromCWD() error {
	start, err := os.Getwd()
	if err != nil {
		return err
	}

	envPath := findDotenv(start)
	if envPath == "" {
		return nil
	}

	f, err := os.Open(envPath)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}

		value = strings.TrimSpace(value)
		value = trimMatchingQuotes(value)
		_ = os.Setenv(key, value)
	}

	return scanner.Err()
}

func findDotenv(start string) string {
	dir := start
	for {
		candidate := filepath.Join(dir, ".env")
		if st, err := os.Stat(candidate); err == nil && !st.IsDir() {
			return candidate
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func trimMatchingQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
		return s[1 : len(s)-1]
	}
	return s
}

func parseEnvFile(path string) ([]string, map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open env file %s: %w", path, err)
	}
	defer f.Close()

	values := make(map[string]string)
	seen := make(map[string]bool)
	keys := make([]string, 0)

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}

		value = strings.TrimSpace(value)
		value = trimMatchingQuotes(value)

		if !seen[key] {
			keys = append(keys, key)
			seen[key] = true
		}
		values[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, nil, fmt.Errorf("failed to read env file %s: %w", path, err)
	}

	return keys, values, nil
}

func envExamplePath(output string) string {
	dir := filepath.Dir(output)
	return filepath.Join(dir, ".env.example")
}

func ensureGitignoreForOutput(output string) error {
	if strings.TrimSpace(output) == "" {
		return nil
	}

	dir := filepath.Dir(output)
	gitignorePath := filepath.Join(dir, ".gitignore")
	rel, err := filepath.Rel(dir, output)
	if err != nil {
		return fmt.Errorf("failed to resolve gitignore entry: %w", err)
	}
	entry := filepath.ToSlash(rel)
	if entry == "." || entry == "" {
		return nil
	}

	var existing []byte
	if b, err := os.ReadFile(gitignorePath); err == nil {
		existing = b
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to read %s: %w", gitignorePath, err)
	}

	if len(existing) > 0 {
		lines := strings.Split(string(existing), "\n")
		for _, line := range lines {
			if strings.TrimSpace(line) == entry {
				return nil
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(gitignorePath), 0o755); err != nil {
		return fmt.Errorf("failed to create gitignore dir: %w", err)
	}

	f, err := os.OpenFile(gitignorePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", gitignorePath, err)
	}
	defer f.Close()

	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return fmt.Errorf("failed to write %s: %w", gitignorePath, err)
		}
	}

	if _, err := f.WriteString(entry + "\n"); err != nil {
		return fmt.Errorf("failed to write %s: %w", gitignorePath, err)
	}

	return nil
}

func writeEnvKeyFile(path string, keys []string, mode os.FileMode) error {
	var buf bytes.Buffer
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		buf.WriteString(key)
		buf.WriteString("=\n")
	}
	if err := os.WriteFile(path, buf.Bytes(), mode); err != nil {
		return fmt.Errorf("failed to write %s: %w", path, err)
	}
	return nil
}

func updateEnvExample(path string, keys []string) ([]string, error) {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if err := writeEnvKeyFile(path, keys, 0o644); err != nil {
				return nil, err
			}
			return keys, nil
		}
		return nil, fmt.Errorf("failed to stat %s: %w", path, err)
	}

	existingKeys, _, err := parseEnvFile(path)
	if err != nil {
		return nil, err
	}

	existing := make(map[string]bool, len(existingKeys))
	for _, key := range existingKeys {
		existing[key] = true
	}

	added := make([]string, 0)
	for _, key := range keys {
		if strings.TrimSpace(key) == "" {
			continue
		}
		if !existing[key] {
			added = append(added, key)
		}
	}
	if len(added) == 0 {
		return added, nil
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read %s: %w", path, err)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		return nil, fmt.Errorf("failed to open %s: %w", path, err)
	}
	defer f.Close()

	if len(b) > 0 && b[len(b)-1] != '\n' {
		if _, err := f.WriteString("\n"); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	for _, key := range added {
		if _, err := f.WriteString(key + "=\n"); err != nil {
			return nil, fmt.Errorf("failed to write %s: %w", path, err)
		}
	}

	return added, nil
}
