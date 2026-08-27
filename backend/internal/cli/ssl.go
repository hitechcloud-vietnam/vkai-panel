package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var sslCmd = &cobra.Command{
	Use:   "ssl",
	Short: "SSL certificate management",
	Long:  `Commands for managing SSL certificates including Let's Encrypt and self-signed certificates.`,
}

var sslRequestCmd = &cobra.Command{
	Use:   "request",
	Short: "Request SSL certificate",
	Run:   runSSLRequest,
}

var sslListCmd = &cobra.Command{
	Use:   "list",
	Short: "List SSL certificates",
	Run:   runSSLList,
}

var sslRenewCmd = &cobra.Command{
	Use:   "renew",
	Short: "Renew SSL certificate",
	Run:   runSSLRenew,
}

var sslRevokeCmd = &cobra.Command{
	Use:   "revoke",
	Short: "Revoke SSL certificate",
	Run:   runSSLRevoke,
}

var sslSelfSignedCmd = &cobra.Command{
	Use:   "self-signed",
	Short: "Create self-signed certificate",
	Run:   runSSLSelfSigned,
}

var (
	sslDomain string
	sslEmail  string
)

func init() {
	sslCmd.AddCommand(sslRequestCmd)
	sslCmd.AddCommand(sslListCmd)
	sslCmd.AddCommand(sslRenewCmd)
	sslCmd.AddCommand(sslRevokeCmd)
	sslCmd.AddCommand(sslSelfSignedCmd)

	sslRequestCmd.Flags().StringVarP(&sslDomain, "domain", "d", "", "Domain name (required)")
	sslRequestCmd.Flags().StringVarP(&sslEmail, "email", "e", "", "Email for Let's Encrypt (required)")
	sslRequestCmd.MarkFlagRequired("domain")
	sslRequestCmd.MarkFlagRequired("email")

	sslRenewCmd.Flags().StringVarP(&sslDomain, "domain", "d", "", "Domain name (required)")
	sslRenewCmd.MarkFlagRequired("domain")

	sslRevokeCmd.Flags().StringVarP(&sslDomain, "domain", "d", "", "Domain name (required)")
	sslRevokeCmd.MarkFlagRequired("domain")

	sslSelfSignedCmd.Flags().StringVarP(&sslDomain, "domain", "d", "", "Domain name (required)")
	sslSelfSignedCmd.MarkFlagRequired("domain")
}

func runSSLRequest(cmd *cobra.Command, args []string) {
	printInfo("Requesting SSL certificate for: %s", sslDomain)

	// Check if certbot is installed
	if _, err := exec.LookPath("certbot"); err != nil {
		printError("certbot is not installed. Install it with: apt install certbot python3-certbot-nginx")
	}

	// Request certificate
	cmdExec := exec.Command("certbot", "certonly", "--nginx", 
		"-d", sslDomain, 
		"--non-interactive", 
		"--agree-tos", 
		"--email", sslEmail)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to request certificate: %v", err)
	}

	printSuccess("SSL certificate requested for: %s", sslDomain)
	printInfo("Certificate location: /etc/letsencrypt/live/%s/", sslDomain)
}

func runSSLList(cmd *cobra.Command, args []string) {
	printInfo("SSL Certificates:")
	fmt.Println()

	// List Let's Encrypt certificates
	certDir := "/etc/letsencrypt/live"
	if _, err := os.Stat(certDir); err == nil {
		entries, err := os.ReadDir(certDir)
		if err != nil {
			printError("Failed to read certificate directory: %v", err)
		}

		for _, entry := range entries {
			if entry.IsDir() {
				certFile := filepath.Join(certDir, entry.Name(), "fullchain.pem")
				if _, err := os.Stat(certFile); err == nil {
					fmt.Printf("  %s\n", entry.Name())
					fmt.Printf("    Certificate: %s\n", certFile)
					fmt.Printf("    Expires: %s\n", getCertExpiry(certFile))
					fmt.Println()
				}
			}
		}
	} else {
		fmt.Println("  No Let's Encrypt certificates found")
	}
}

func getCertExpiry(certFile string) string {
	cmd := exec.Command("openssl", "x509", "-enddate", "-noout", "-in", certFile)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	return string(output)
}

func runSSLRenew(cmd *cobra.Command, args []string) {
	printInfo("Renewing SSL certificate for: %s", sslDomain)

	cmdExec := exec.Command("certbot", "renew", "--cert-name", sslDomain)
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to renew certificate: %v", err)
	}

	printSuccess("Certificate renewed for: %s", sslDomain)
}

func runSSLRevoke(cmd *cobra.Command, args []string) {
	printInfo("Revoking SSL certificate for: %s", sslDomain)

	cmdExec := exec.Command("certbot", "revoke", "--cert-name", sslDomain, "--non-interactive")
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to revoke certificate: %v", err)
	}

	printSuccess("Certificate revoked for: %s", sslDomain)
}

func runSSLSelfSigned(cmd *cobra.Command, args []string) {
	printInfo("Creating self-signed certificate for: %s", sslDomain)

	certDir := "/opt/vkai-panel/ssl"
	if err := os.MkdirAll(certDir, 0755); err != nil {
		printError("Failed to create SSL directory: %v", err)
	}

	certFile := filepath.Join(certDir, sslDomain+".crt")
	keyFile := filepath.Join(certDir, sslDomain+".key")

	// Generate self-signed certificate
	cmdExec := exec.Command("openssl", "req", "-x509", "-nodes", "-days", "365",
		"-newkey", "rsa:2048",
		"-keyout", keyFile,
		"-out", certFile,
		"-subj", fmt.Sprintf("/CN=%s/O=vKAI Panel/C=VN", sslDomain))
	cmdExec.Stdout = os.Stdout
	cmdExec.Stderr = os.Stderr

	if err := cmdExec.Run(); err != nil {
		printError("Failed to create certificate: %v", err)
	}

	printSuccess("Self-signed certificate created:")
	fmt.Printf("  Certificate: %s\n", certFile)
	fmt.Printf("  Key: %s\n", keyFile)
	fmt.Printf("  Expires: %s\n", time.Now().AddDate(1, 0, 0).Format("2006-01-02"))
}
