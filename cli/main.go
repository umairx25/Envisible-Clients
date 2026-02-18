package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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
	defaultDashboardURL = "https://envisible.netlify.app"
	pollWaitTimeout     = 120 * time.Second
	defaultPollDelay    = 5 * time.Second
)

type Config struct {
	BaseURL      string
	DashboardURL string
	SessionPath  string
	CIToken      string
}

type Session struct {
	AccessToken  string                 `json:"access_token"`
	RefreshToken string                 `json:"refresh_token"`
	ExpiresAt    int64                  `json:"expires_at"`
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
	ProjectID   string `json:"project_id"`
	Name        string `json:"name"`
	OwnerID     string `json:"owner_id"`
	CITokenSet  bool   `json:"ci_token_set"`
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
}

type ProjectMembersResponse struct {
	ProjectID string          `json:"project_id"`
	OwnerID   *string         `json:"owner_id"`
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
		_, err := ensureSession(cfg)
		if err != nil {
			return err
		}
		fmt.Println("Logged in successfully.")
		return nil
	case "logout":
		if err := os.Remove(cfg.SessionPath); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return errors.New("already logged out")
			}
			return fmt.Errorf("failed to remove session: %w", err)
		}
		fmt.Println("Logged out successfully.")
		return nil
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
	fmt.Println("  envis secret-get --project-id <uuid> --name <key>")
	fmt.Println("  envis secret-set --project-id <uuid> --name <key> --value <value>")
	fmt.Println("  envis secret-delete --project-id <uuid> --name <key>")
	fmt.Println("  envis projects")
	fmt.Println("  envis project-create --name <name>")
	fmt.Println("  envis project-rename --project-id <uuid> --name <new-name>")
	fmt.Println("  envis project-delete --project-id <uuid>")
	fmt.Println("  envis project-members --project-id <uuid>")
	fmt.Println("  envis project-member-remove --project-id <uuid> --user-id <uuid>")
	fmt.Println("  envis get-many [--project-id <uuid>] <secret1> <secret2> ...")
	fmt.Println("  envis invites")
	fmt.Println("  envis invite-respond --invite-id <uuid> --accept|--reject")
	fmt.Println("  envis invite-create --project-id <uuid> --email <user@example.com>")
	fmt.Println("  envis status")
	fmt.Println("  envis ci-token-generate --project-id <uuid>")
	fmt.Println("  envis ci-token-reset --project-id <uuid>")
	fmt.Println("  envis ci-token-verify --project-id <uuid> --token <token>")
	fmt.Println("  envis login")
	fmt.Println("  envis logout")
}

func runPull(cfg Config, args []string) error {
	fs := flag.NewFlagSet("pull", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	output := fs.String("output", ".env", "Output env file path")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	secrets, err := getAllSecrets(client, cfg, projectID)
	if err != nil {
		return err
	}

	sort.Slice(secrets, func(i, j int) bool {
		return secrets[i].Name < secrets[j].Name
	})

	var buf bytes.Buffer
	for _, secret := range secrets {
		if strings.TrimSpace(secret.Name) == "" {
			continue
		}
		buf.WriteString(secret.Name)
		buf.WriteString("=")
		buf.WriteString(formatEnvValue(secret.Value))
		buf.WriteString("\n")
	}

	if err := os.WriteFile(*output, buf.Bytes(), 0o600); err != nil {
		return fmt.Errorf("failed to write %s: %w", *output, err)
	}

	fmt.Printf("Wrote %d secret(s) to %s\n", len(secrets), *output)
	return nil
}

func runGetMany(cfg Config, args []string) error {
	fs := flag.NewFlagSet("get-many", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	rawNames := fs.Args()
	if len(rawNames) == 0 {
		return errors.New("get-many requires at least one secret name")
	}

	names := make([]string, 0, len(rawNames))
	for _, name := range rawNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return errors.New("get-many requires non-empty secret names")
		}
		names = append(names, trimmed)
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	keys, values, err := parseEnvFile(*envFile)
	if err != nil {
		return err
	}
	if len(keys) == 0 {
		return fmt.Errorf("no secrets found in %s", *envFile)
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	for _, name := range keys {
		if err := upsertSecret(client, cfg, projectID, name, values[name]); err != nil {
			return fmt.Errorf("failed to push %q: %w", name, err)
		}
	}

	fmt.Printf("Pushed %d secret(s) from %s\n", len(keys), *envFile)
	return nil
}

func runSecretNames(cfg Config, args []string) error {
	fs := flag.NewFlagSet("secret-names", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	client, err := newHTTPClient(cfg)
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		return errors.New("missing secret name: pass --name")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		return errors.New("missing secret name: pass --name")
	}
	if !valueProvided {
		return errors.New("missing secret value: pass --value")
	}
	secretValue := *value

	if cfg.CIToken != "" {
		return errors.New("secret-set command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	if err := upsertSecret(client, cfg, projectID, secretName, secretValue); err != nil {
		return err
	}

	fmt.Printf("Secret %s updated.\n", secretName)
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	secretName := strings.TrimSpace(*name)
	if secretName == "" {
		return errors.New("missing secret name: pass --name")
	}

	if cfg.CIToken != "" {
		return errors.New("secret-delete command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	if err := deleteSecret(client, cfg, projectID, secretName); err != nil {
		return err
	}

	fmt.Printf("Secret %s deleted.\n", secretName)
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

	userID, err := getCurrentUserID(client, cfg)
	if err != nil {
		return err
	}

	if len(projects) == 0 {
		fmt.Println("No projects found.")
		return nil
	}

	sort.Slice(projects, func(i, j int) bool {
		return strings.ToLower(projects[i].Name) < strings.ToLower(projects[j].Name)
	})

	for _, project := range projects {
		role := "member"
		if strings.TrimSpace(project.OwnerID) == userID {
			role = "owner"
		}
		ciTokenStatus := "no"
		if project.CITokenSet {
			ciTokenStatus = "yes"
		}
		fmt.Printf("%s\t%s\t%s\t%s\n", project.ProjectID, project.Name, role, ciTokenStatus)
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
		return errors.New("missing project name: pass --name")
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

	fmt.Printf("Project created: %s\t%s\n", project.ProjectID, project.Name)
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	newName := strings.TrimSpace(*name)
	if newName == "" {
		return errors.New("missing project name: pass --name")
	}

	if cfg.CIToken != "" {
		return errors.New("project-rename command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	project, err := renameProject(client, cfg, projectID, newName)
	if err != nil {
		return err
	}

	fmt.Printf("Project renamed: %s\t%s\n", project.ProjectID, project.Name)
	return nil
}

func runProjectDelete(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-delete", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-delete command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	if err := deleteProject(client, cfg, projectID); err != nil {
		return err
	}

	fmt.Printf("Project %s deleted.\n", projectID)
	return nil
}

func runProjectMembers(cfg Config, args []string) error {
	fs := flag.NewFlagSet("project-members", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("project-members command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
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
		name := strings.TrimSpace(member.Name)
		email := strings.TrimSpace(member.Email)
		fmt.Printf("%s\t%s\t%s\n", strings.TrimSpace(member.ID), name, email)
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	userID := strings.TrimSpace(*userIDFlag)
	if userID == "" {
		return errors.New("missing user id: pass --user-id")
	}

	if cfg.CIToken != "" {
		return errors.New("project-member-remove command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	if err := removeProjectMember(client, cfg, projectID, userID); err != nil {
		return err
	}

	fmt.Printf("Removed member %s from project %s.\n", userID, projectID)
	return nil
}

func runCiTokenGenerate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("ci-token-generate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-generate command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	token, err := generateCIToken(client, cfg, projectID)
	if err != nil {
		return err
	}

	fmt.Println("CI token generated (store this now, it won't be shown again):")
	fmt.Println(token)
	return nil
}

func runCiTokenReset(cfg Config, args []string) error {
	fs := flag.NewFlagSet("ci-token-reset", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-reset command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	token, err := generateCIToken(client, cfg, projectID)
	if err != nil {
		return err
	}

	fmt.Println("CI token reset (store this now, it won't be shown again):")
	fmt.Println(token)
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

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	if !tokenProvided {
		return errors.New("missing token: pass --token")
	}
	if strings.TrimSpace(*token) == "" {
		return errors.New("token cannot be empty")
	}

	if cfg.CIToken != "" {
		return errors.New("ci-token-verify command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	ok, err := verifyCIToken(client, cfg, projectID, *token)
	if err != nil {
		return err
	}

	if ok {
		fmt.Println("CI token is valid.")
	} else {
		fmt.Println("CI token is invalid.")
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
		fmt.Printf("%s\t%s\t%s\t%s\n",
			strings.TrimSpace(invite.ID),
			strings.TrimSpace(invite.InviterName),
			strings.TrimSpace(invite.InviterEmail),
			strings.TrimSpace(invite.ProjectName),
		)
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

	inviteID := strings.TrimSpace(*inviteIDFlag)
	if inviteID == "" {
		return errors.New("missing invite id: pass --invite-id")
	}
	if *accept == *reject {
		return errors.New("choose exactly one of --accept or --reject")
	}

	if cfg.CIToken != "" {
		return errors.New("invite-respond command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	action := "accept"
	if *reject {
		action = "reject"
	}

	updated, err := respondToInvite(client, cfg, inviteID, action)
	if err != nil {
		return err
	}

	fmt.Printf("Invite %s %s.\n", updated.ID, updated.Status)
	return nil
}

func runInviteCreate(cfg Config, args []string) error {
	fs := flag.NewFlagSet("invite-create", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	projectIDFlag := fs.String("project-id", "", "Project UUID")
	email := fs.String("email", "", "Recipient email")
	if err := fs.Parse(args); err != nil {
		return err
	}

	projectID, err := resolveProjectID(*projectIDFlag)
	if err != nil {
		return err
	}

	recipient := strings.TrimSpace(*email)
	if recipient == "" {
		return errors.New("missing email: pass --email")
	}

	if cfg.CIToken != "" {
		return errors.New("invite-create command requires user auth (ENVIS_CI_TOKEN is not supported)")
	}

	client, err := newHTTPClient(cfg)
	if err != nil {
		return err
	}

	project, err := getProjectByID(client, cfg, projectID)
	if err != nil {
		return err
	}
	if project == nil {
		return errors.New("project not found or not accessible")
	}

	userID, err := getCurrentUserID(client, cfg)
	if err != nil {
		return err
	}

	if strings.TrimSpace(project.OwnerID) != userID {
		return errors.New("only the project owner can create invites")
	}

	invite, err := createInvite(client, cfg, projectID, recipient)
	if err != nil {
		return err
	}

	fmt.Printf("Invite created: %s\n", invite.ID)
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
		return nil
	}

	session, err := loadSession(cfg.SessionPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Println("Auth mode: user")
			fmt.Println("Signed in: no")
			return nil
		}
		return err
	}

	if session.AccessToken == "" && session.RefreshToken == "" {
		fmt.Println("Auth mode: user")
		fmt.Println("Signed in: no (invalid session)")
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
		fmt.Printf("User ID: %s\n", strings.TrimSpace(sessionUserID(session)))
		fmt.Printf("User info unavailable: %v\n", err)
		return nil
	}

	fmt.Println("Auth mode: user")
	fmt.Println("Signed in: yes")
	if strings.TrimSpace(profile.Email) != "" {
		fmt.Printf("Email: %s\n", strings.TrimSpace(profile.Email))
	}
	if strings.TrimSpace(profile.ID) != "" {
		fmt.Printf("User ID: %s\n", strings.TrimSpace(profile.ID))
	}
	if strings.TrimSpace(profile.Name) != "" {
		fmt.Printf("Name: %s\n", strings.TrimSpace(profile.Name))
	}
	return nil
}

func resolveProjectID(flagValue string) (string, error) {
	if strings.TrimSpace(flagValue) != "" {
		return strings.TrimSpace(flagValue), nil
	}

	if value := strings.TrimSpace(os.Getenv("ENVIS_PROJECT_ID")); value != "" {
		return value, nil
	}

	return "", errors.New("missing project id: pass --project-id or set ENVIS_PROJECT_ID")
}

func newConfig() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Config{}, fmt.Errorf("failed to resolve home dir: %w", err)
	}

	baseURL := strings.TrimSpace(os.Getenv("ENVIS_API_URL"))
	if baseURL == "" {
		baseURL = defaultAPIURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	dashURL := strings.TrimSpace(os.Getenv("ENVIS_DASH_URL"))
	if dashURL == "" {
		dashURL = defaultDashboardURL
	}
	dashURL = strings.TrimRight(dashURL, "/")

	return Config{
		BaseURL:      baseURL,
		DashboardURL: dashURL,
		SessionPath:  filepath.Join(home, ".envis", "session.json"),
		CIToken:      strings.TrimSpace(os.Getenv("ENVIS_CI_TOKEN")),
	}, nil
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

	endpoint := cfg.BaseURL + "/v1/auth/refresh"
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

	endpoint := cfg.BaseURL + "/v1/auth/" + url.PathEscape(deviceCode)
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
			return Session{}, fmt.Errorf("auth endpoint returned invalid JSON (status %d)", resp.StatusCode)
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects", cfg.BaseURL)
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
	endpoint := fmt.Sprintf("%s/v1/me", cfg.BaseURL)
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}

	var user CurrentUser
	if err := json.Unmarshal(respBody, &user); err != nil {
		return "", fmt.Errorf("invalid /v1/me response: %w", err)
	}
	user.ID = strings.TrimSpace(user.ID)
	if user.ID == "" {
		return "", errors.New("unable to determine authenticated user id")
	}
	return user.ID, nil
}

func getCurrentUserProfile(client *http.Client, cfg Config) (CurrentUserProfile, error) {
	endpoint := fmt.Sprintf("%s/v1/me", cfg.BaseURL)
	respBody, err := doJSONRequest(client, http.MethodGet, endpoint, nil)
	if err != nil {
		return CurrentUserProfile{}, err
	}

	var user CurrentUserProfile
	if err := json.Unmarshal(respBody, &user); err != nil {
		return CurrentUserProfile{}, fmt.Errorf("invalid /v1/me response: %w", err)
	}
	return user, nil
}

func getSecret(client *http.Client, cfg Config, projectID string, name string) (string, error) {
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/%s", cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(name))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/all", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/batch", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/%s", cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(name))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/secrets/%s", cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(name))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func listProjectMembers(client *http.Client, cfg Config, projectID string) (ProjectMembersResponse, error) {
	endpoint := fmt.Sprintf("%s/v1/projects/%s/members", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/members/%s", cfg.BaseURL, url.PathEscape(projectID), url.PathEscape(userID))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func listMyInvites(client *http.Client, cfg Config) ([]Invite, error) {
	endpoint := fmt.Sprintf("%s/v1/me/invites", cfg.BaseURL)
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

	endpoint := fmt.Sprintf("%s/v1/invites/%s/%s", cfg.BaseURL, url.PathEscape(inviteID), action)
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

func createInvite(client *http.Client, cfg Config, projectID, email string) (Invite, error) {
	endpoint := fmt.Sprintf("%s/v1/projects/%s/invite", cfg.BaseURL, url.PathEscape(projectID))
	payload := InviteCreateRequest{Email: strings.TrimSpace(email)}
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
	endpoint := fmt.Sprintf("%s/v1/projects", cfg.BaseURL)
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s", cfg.BaseURL, url.PathEscape(projectID))
	if _, err := doJSONRequest(client, http.MethodDelete, endpoint, nil); err != nil {
		return err
	}
	return nil
}

func generateCIToken(client *http.Client, cfg Config, projectID string) (string, error) {
	endpoint := fmt.Sprintf("%s/v1/projects/%s/ci-token", cfg.BaseURL, url.PathEscape(projectID))
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
	endpoint := fmt.Sprintf("%s/v1/projects/%s/ci-token/verify", cfg.BaseURL, url.PathEscape(projectID))
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
