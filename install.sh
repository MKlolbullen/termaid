#!/usr/bin/env bash
#
# Termaid Enhanced Installer
# Comprehensive bug bounty tool installation and setup
#
set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
PURPLE='\033[0;35m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
INSTALL_DIR="/opt/termaid-tools"
GO_VERSION="1.22.4"
NODE_VERSION="20"
PYTHON_REQUIREMENTS="requirements.txt"

# Logging
LOG_FILE="install.log"
exec > >(tee -a "$LOG_FILE")
exec 2>&1

echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
echo -e "${CYAN}║       Termaid Enhanced Installer     ║${NC}"
echo -e "${CYAN}║    Bug Bounty Automation Platform    ║${NC}"
echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
echo ""

# Helper functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

log_step() {
    echo -e "${BLUE}[STEP]${NC} $1"
}

check_root() {
    if [[ $EUID -eq 0 ]]; then
        log_error "This script should not be run as root for security reasons"
        exit 1
    fi
}

detect_os() {
    if [[ -f /etc/os-release ]]; then
        . /etc/os-release
        OS=$ID
        VER=$VERSION_ID
    else
        log_error "Cannot detect OS. Please install manually."
        exit 1
    fi
    
    log_info "Detected OS: $OS $VER"
}

install_prerequisites() {
    log_step "Installing system prerequisites..."
    
    case $OS in
        ubuntu|debian)
            sudo apt-get update -qq
            sudo apt-get install -y \
                curl wget git jq unzip zip \
                build-essential software-properties-common \
                python3 python3-pip python3-venv \
                nodejs npm \
                ca-certificates gnupg lsb-release \
                apt-transport-https
            ;;
        fedora|centos|rhel)
            if command -v dnf >/dev/null; then
                PKG_MANAGER="dnf"
            else
                PKG_MANAGER="yum"
            fi
            sudo $PKG_MANAGER install -y \
                curl wget git jq unzip zip \
                gcc gcc-c++ make \
                python3 python3-pip \
                nodejs npm \
                ca-certificates
            ;;
        arch|manjaro)
            sudo pacman -Sy --needed --noconfirm \
                curl wget git jq unzip zip \
                base-devel \
                python python-pip \
                nodejs npm
            ;;
        *)
            log_error "Unsupported OS: $OS"
            log_info "Please install manually: curl wget git jq unzip python3 nodejs npm build-tools"
            exit 1
            ;;
    esac
    
    log_info "System prerequisites installed successfully"
}

install_go() {
    if command -v go >/dev/null 2>&1; then
        CURRENT_GO_VERSION=$(go version | awk '{print $3}' | sed 's/go//')
        log_info "Go $CURRENT_GO_VERSION already installed"
        if [[ "$CURRENT_GO_VERSION" < "1.22" ]]; then
            log_warn "Go version is old, updating..."
        else
            return 0
        fi
    fi
    
    log_step "Installing Go $GO_VERSION..."
    
    # Detect architecture
    ARCH=$(uname -m)
    case $ARCH in
        x86_64) GOARCH="amd64" ;;
        aarch64|arm64) GOARCH="arm64" ;;
        armv7l) GOARCH="armv6l" ;;
        *) log_error "Unsupported architecture: $ARCH"; exit 1 ;;
    esac
    
    GO_TARBALL="go${GO_VERSION}.linux-${GOARCH}.tar.gz"
    
    wget -q "https://go.dev/dl/${GO_TARBALL}" -O "/tmp/${GO_TARBALL}"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${GO_TARBALL}"
    rm "/tmp/${GO_TARBALL}"
    
    # Add to PATH
    echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
    echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.zshrc 2>/dev/null || true
    export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
    
    log_info "Go $GO_VERSION installed successfully"
}

setup_directories() {
    log_step "Setting up directories..."
    
    mkdir -p ~/go/bin
    mkdir -p ~/.config/termaid
    mkdir -p ~/.local/share/termaid/wordlists
    mkdir -p ~/.local/share/termaid/templates
    
    log_info "Directories created successfully"
}

install_go_tools() {
    log_step "Installing Go-based security tools..."
    
    declare -A go_tools=(
        ["subfinder"]="github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest"
        ["httpx"]="github.com/projectdiscovery/httpx/cmd/httpx@latest"
        ["nuclei"]="github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest"
        ["naabu"]="github.com/projectdiscovery/naabu/v2/cmd/naabu@latest"
        ["dnsx"]="github.com/projectdiscovery/dnsx/cmd/dnsx@latest"
        ["chaos"]="github.com/projectdiscovery/chaos-client/cmd/chaos@latest"
        ["uncover"]="github.com/projectdiscovery/uncover/cmd/uncover@latest"
        ["alterx"]="github.com/projectdiscovery/alterx/cmd/alterx@latest"
        ["katana"]="github.com/projectdiscovery/katana/cmd/katana@latest"
        ["ffuf"]="github.com/ffuf/ffuf@latest"
        ["assetfinder"]="github.com/tomnomnom/assetfinder@latest"
        ["gau"]="github.com/lc/gau@latest"
        ["gauplus"]="github.com/bp0lr/gauplus@latest"
        ["gf"]="github.com/tomnomnom/gf@latest"
        ["dalfox"]="github.com/hahwul/dalfox/v2@latest"
        ["kxss"]="github.com/Emoe/kxss@latest"
        ["subjack"]="github.com/haccer/subjack@latest"
        ["subzy"]="github.com/PentestPad/subzy@latest"
        ["hakrawler"]="github.com/hakluke/hakrawler@latest"
        ["gobuster"]="github.com/OJ/gobuster/v3@latest"
        ["amass"]="github.com/owasp-amass/amass/v4/...@latest"
        ["aquatone"]="github.com/michenriksen/aquatone@latest"
        ["gowitness"]="github.com/sensepost/gowitness@latest"
    )
    
    failed_tools=()
    
    for tool in "${!go_tools[@]}"; do
        echo -n "  Installing $tool... "
        if go install "${go_tools[$tool]}" >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            failed_tools+=("$tool")
        fi
    done
    
    if [[ ${#failed_tools[@]} -gt 0 ]]; then
        log_warn "Failed to install: ${failed_tools[*]}"
    fi
    
    log_info "Go tools installation completed"
}

install_python_tools() {
    log_step "Installing Python-based security tools..."
    
    # Create virtual environment
    python3 -m venv ~/.local/share/termaid/venv
    source ~/.local/share/termaid/venv/bin/activate
    
    # Upgrade pip
    pip install --upgrade pip setuptools wheel
    
    declare -a python_tools=(
        "arjun"
        "waymore"
        "parameth"
        "trufflehog"
        "xsrfprobe"
        "cloakquest3r"
        "photon-scanner"
        "LinkFinder"
        "smuggler"
        "corsy"
        "ssrfire"
        "xsstrike"
    )
    
    failed_tools=()
    
    for tool in "${python_tools[@]}"; do
        echo -n "  Installing $tool... "
        if pip install "$tool" >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            failed_tools+=("$tool")
        fi
    done
    
    # Create activation script
    cat > ~/.local/share/termaid/activate_tools.sh << 'EOF'
#!/bin/bash
source ~/.local/share/termaid/venv/bin/activate
export PATH=$PATH:~/.local/share/termaid/venv/bin
EOF
    chmod +x ~/.local/share/termaid/activate_tools.sh
    
    if [[ ${#failed_tools[@]} -gt 0 ]]; then
        log_warn "Failed to install Python tools: ${failed_tools[*]}"
    fi
    
    log_info "Python tools installation completed"
}

install_custom_tools() {
    log_step "Installing custom and compiled tools..."
    
    cd /tmp
    
    # MassDNS
    if ! command -v massdns >/dev/null; then
        echo -n "  Installing massdns... "
        if git clone https://github.com/blechschmidt/massdns.git >/dev/null 2>&1 && \
           cd massdns && make >/dev/null 2>&1 && \
           sudo cp bin/massdns /usr/local/bin/; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
        cd /tmp
    fi
    
    # Cariddi
    if ! command -v cariddi >/dev/null; then
        echo -n "  Installing cariddi... "
        if git clone https://github.com/edoardottt/cariddi.git >/dev/null 2>&1 && \
           cd cariddi && go build -o cariddi >/dev/null 2>&1 && \
           sudo mv cariddi /usr/local/bin/; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
        cd /tmp
    fi
    
    # Kiterunner
    if ! command -v kr >/dev/null; then
        echo -n "  Installing kiterunner... "
        if git clone https://github.com/assetnote/kiterunner.git >/dev/null 2>&1 && \
           cd kiterunner && make build >/dev/null 2>&1 && \
           sudo cp dist/kr /usr/local/bin/; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
        cd /tmp
    fi
    
    log_info "Custom tools installation completed"
}

download_wordlists() {
    log_step "Downloading essential wordlists..."
    
    WORDLIST_DIR="$HOME/.local/share/termaid/wordlists"
    
    # SecLists
    if [[ ! -d "$WORDLIST_DIR/SecLists" ]]; then
        echo -n "  Downloading SecLists... "
        if git clone https://github.com/danielmiessler/SecLists.git "$WORDLIST_DIR/SecLists" >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
    fi
    
    # Common wordlists
    mkdir -p "$WORDLIST_DIR/common"
    
    declare -A wordlists=(
        ["subdomains.txt"]="https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/DNS/subdomains-top1million-5000.txt"
        ["directories.txt"]="https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/directory-list-2.3-medium.txt"
        ["parameters.txt"]="https://raw.githubusercontent.com/danielmiessler/SecLists/master/Discovery/Web-Content/burp-parameter-names.txt"
    )
    
    for name in "${!wordlists[@]}"; do
        if [[ ! -f "$WORDLIST_DIR/common/$name" ]]; then
            echo -n "  Downloading $name... "
            if wget -q "${wordlists[$name]}" -O "$WORDLIST_DIR/common/$name"; then
                echo -e "${GREEN}✓${NC}"
            else
                echo -e "${RED}✗${NC}"
            fi
        fi
    done
    
    log_info "Wordlists downloaded successfully"
}

install_nuclei_templates() {
    log_step "Installing Nuclei templates..."
    
    if command -v nuclei >/dev/null; then
        echo -n "  Updating nuclei templates... "
        if nuclei -update-templates >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
        fi
    fi
    
    log_info "Nuclei templates installed"
}

build_termaid() {
    log_step "Building Termaid..."
    
    if [[ ! -f "go.mod" ]]; then
        log_error "Not in termaid directory. Please run from the project root."
        exit 1
    fi
    
    echo -n "  Building termaid binary... "
    if go mod tidy >/dev/null 2>&1 && go build -o termaid ./cmd/termaid >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
        chmod +x termaid
    else
        echo -e "${RED}✗${NC}"
        log_error "Failed to build termaid"
        exit 1
    fi
    
    # Install globally
    sudo cp termaid /usr/local/bin/
    
    log_info "Termaid built and installed successfully"
}

setup_aliases() {
    log_step "Setting up aliases and shortcuts..."
    
    # Add termaid aliases
    cat >> ~/.bashrc << 'EOF'

# Termaid aliases
alias termaid-activate='source ~/.local/share/termaid/activate_tools.sh'
alias termaid-update='cd $(dirname $(which termaid)) && git pull && go build -o termaid ./cmd/termaid && sudo cp termaid /usr/local/bin/'

# Common tool aliases
alias subdomains='subfinder -silent'
alias ports='naabu -silent'
alias urls='gau --blacklist png,jpg,gif,jpeg,swf,woff,svg,pdf,css,js'
alias nuclei-scan='nuclei -silent'
EOF
    
    # Add to zsh if present
    if [[ -f ~/.zshrc ]]; then
        cat >> ~/.zshrc << 'EOF'

# Termaid aliases
alias termaid-activate='source ~/.local/share/termaid/activate_tools.sh'
alias termaid-update='cd $(dirname $(which termaid)) && git pull && go build -o termaid ./cmd/termaid && sudo cp termaid /usr/local/bin/'

# Common tool aliases
alias subdomains='subfinder -silent'
alias ports='naabu -silent'
alias urls='gau --blacklist png,jpg,gif,jpeg,swf,woff,svg,pdf,css,js'
alias nuclei-scan='nuclei -silent'
EOF
    fi
    
    log_info "Aliases configured successfully"
}

verify_installation() {
    log_step "Verifying installation..."
    
    declare -a required_tools=(
        "go" "python3" "git" "curl" "wget"
        "subfinder" "httpx" "nuclei" "termaid"
    )
    
    missing_tools=()
    
    for tool in "${required_tools[@]}"; do
        echo -n "  Checking $tool... "
        if command -v "$tool" >/dev/null 2>&1; then
            echo -e "${GREEN}✓${NC}"
        else
            echo -e "${RED}✗${NC}"
            missing_tools+=("$tool")
        fi
    done
    
    if [[ ${#missing_tools[@]} -gt 0 ]]; then
        log_error "Missing required tools: ${missing_tools[*]}"
        return 1
    fi
    
    # Test termaid
    echo -n "  Testing termaid... "
    if termaid --help >/dev/null 2>&1; then
        echo -e "${GREEN}✓${NC}"
    else
        echo -e "${RED}✗${NC}"
        log_error "Termaid test failed"
        return 1
    fi
    
    log_info "Installation verification completed successfully"
    return 0
}

create_config() {
    log_step "Creating configuration files..."
    
    # Create main config
    cat > ~/.config/termaid/config.yaml << EOF
# Termaid Configuration
version: "1.0"

# Default settings
defaults:
  concurrency: 10
  timeout: 300
  output_format: "txt"
  
# Tool paths
tools:
  wordlists: "$HOME/.local/share/termaid/wordlists"
  nuclei_templates: "$HOME/nuclei-templates"
  
# API keys (set your own)
api_keys:
  shodan: ""
  censys: ""
  chaos: ""
  virustotal: ""
EOF
    
    log_info "Configuration files created"
}

cleanup() {
    log_step "Cleaning up temporary files..."
    
    rm -rf /tmp/massdns /tmp/cariddi /tmp/kiterunner
    rm -f /tmp/go*.tar.gz
    
    log_info "Cleanup completed"
}

print_summary() {
    echo ""
    echo -e "${CYAN}╔══════════════════════════════════════╗${NC}"
    echo -e "${CYAN}║        Installation Complete!        ║${NC}"
    echo -e "${CYAN}╚══════════════════════════════════════╝${NC}"
    echo ""
    echo -e "${GREEN}✓ Termaid installed successfully${NC}"
    echo -e "${GREEN}✓ Bug bounty tools installed${NC}"
    echo -e "${GREEN}✓ Wordlists downloaded${NC}"
    echo -e "${GREEN}✓ Configuration created${NC}"
    echo ""
    echo -e "${YELLOW}Next steps:${NC}"
    echo "1. Restart your terminal or run: source ~/.bashrc"
    echo "2. Activate Python tools: termaid-activate"
    echo "3. Run termaid: termaid"
    echo "4. Configure API keys in: ~/.config/termaid/config.yaml"
    echo ""
    echo -e "${BLUE}Quick start:${NC}"
    echo "  termaid                    # Launch the TUI"
    echo "  termaid-activate          # Activate Python tools"
    echo "  termaid-update            # Update termaid"
    echo ""
    echo -e "${PURPLE}Documentation:${NC} https://github.com/MKlolbullen/termaid"
    echo -e "${PURPLE}Issues:${NC} https://github.com/MKlolbullen/termaid/issues"
    echo ""
}

# Main installation flow
main() {
    check_root
    detect_os
    install_prerequisites
    install_go
    setup_directories
    install_go_tools
    install_python_tools
    install_custom_tools
    download_wordlists
    install_nuclei_templates
    build_termaid
    setup_aliases
    create_config
    cleanup
    
    if verify_installation; then
        print_summary
    else
        log_error "Installation verification failed. Check the log for details."
        exit 1
    fi
}

# Handle interruption
trap 'log_error "Installation interrupted"; exit 1' INT TERM

# Run main installation
main "$@"