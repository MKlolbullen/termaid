#!/usr/bin/env bash
#
# BB-Runner install.sh
# Installs Go, Python, and all bug bounty tools from tools.yaml.
#
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}[*] BB-Runner Installer starting...${NC}"

# --- OS detection ---
if command -v apt-get >/dev/null; then
    PKG="apt-get"
    SUDO="sudo"
elif command -v pacman >/dev/null; then
    PKG="pacman"
    SUDO="sudo"
else
    echo -e "${YELLOW}Unsupported system. Please install packages manually.${NC}"
    exit 1
fi

# --- Basic prerequisites ---
echo -e "${GREEN}[*] Installing build essentials, git, curl, python3, pip...${NC}"
if [ "$PKG" = "apt-get" ]; then
    $SUDO apt-get update
    $SUDO apt-get install -y build-essential git curl wget python3 python3-pip python3-venv jq unzip
elif [ "$PKG" = "pacman" ]; then
    $SUDO pacman -Sy --needed --noconfirm base-devel git curl wget python python-pip jq unzip
fi

# --- Go install if missing ---
if ! command -v go >/dev/null; then
    echo -e "${YELLOW}[!] Go not found. Installing Go 1.22...${NC}"
    curl -LO https://go.dev/dl/go1.22.4.linux-amd64.tar.gz
    $SUDO rm -rf /usr/local/go
    $SUDO tar -C /usr/local -xzf go1.22.4.linux-amd64.tar.gz
    echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.zshrc
    export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
fi

# --- Node.js (for tools like kiterunner, openredirex, waymore) ---
if ! command -v node >/dev/null; then
    echo -e "${GREEN}[*] Installing Node.js (for kiterunner, openredirex, waymore)...${NC}"
    curl -fsSL https://deb.nodesource.com/setup_lts.x | $SUDO bash -
    $SUDO $PKG install -y nodejs
fi

echo -e "${GREEN}[*] Installing Charm.sh libraries...${NC}"
go install github.com/charmbracelet/bubbletea@latest
go install github.com/charmbracelet/bubbles@latest
go install github.com/charmbracelet/lipgloss@latest
go install github.com/charmbracelet/glow@latest

echo -e "${GREEN}[*] Compiling BB-Runner...${NC}"
go mod tidy
go build -o bb-runner ./cmd/bb-runner || go build -o bb-runner

# --- Go tools (direct install) ---
echo -e "${GREEN}[*] Installing main Go-based recon tools...${NC}"
go install github.com/projectdiscovery/subfinder/v2/cmd/subfinder@latest
go install github.com/tomnomnom/assetfinder@latest
go install github.com/projectdiscovery/chaos-client/cmd/chaos@latest
go install github.com/projectdiscovery/httpx/cmd/httpx@latest
go install github.com/projectdiscovery/naabu/v2/cmd/naabu@latest
go install github.com/projectdiscovery/dnsx/cmd/dnsx@latest
go install github.com/projectdiscovery/nuclei/v3/cmd/nuclei@latest
go install github.com/hakluke/hakrawler@latest
go install github.com/projectdiscovery/alterx/cmd/alterx@latest
go install github.com/hakluke/gf@latest
go install github.com/lc/gau@latest
go install github.com/bp0lr/gauplus@latest
go install github.com/projectdiscovery/uncover/cmd/uncover@latest
go install github.com/projectdiscovery/subjack/cmd/subjack@latest
go install github.com/hakluke/subzy@latest
go install github.com/ffuf/ffuf@latest
go install github.com/Emoe/kxss@latest
go install github.com/lc/gau@latest

# --- More Go tools and custom sources ---
if ! command -v gobuster >/dev/null; then
    go install github.com/OJ/gobuster/v3@latest
fi
if ! command -v amass >/dev/null; then
    go install github.com/owasp-amass/amass/v4/...@latest
fi
if ! command -v massdns >/dev/null; then
    git clone https://github.com/blechschmidt/massdns.git /tmp/massdns
    cd /tmp/massdns && make && $SUDO cp bin/massdns /usr/local/bin && cd -
    rm -rf /tmp/massdns
fi
if ! command -v dalfox >/dev/null; then
    go install github.com/hahwul/dalfox/v2@latest
fi
if ! command -v arjun >/dev/null; then
    pip3 install arjun
fi
if ! command -v gf >/dev/null; then
    go install github.com/tomnomnom/gf@latest
fi

# --- Tools that are typically pip, npm, or git-clone based ---
echo -e "${GREEN}[*] Installing Python, Node, or git-based tools...${NC}"
pip3 install xsrfprobe trufflehog waymore parameth photon openredirex

# cariddi (Go-based, has prebuilt binary)
if ! command -v cariddi >/dev/null; then
    git clone https://github.com/edoardottt/cariddi /tmp/cariddi
    cd /tmp/cariddi
    go build -o cariddi
    $SUDO mv cariddi /usr/local/bin
    cd - && rm -rf /tmp/cariddi
fi

# cloakquest3r (Python)
pip3 install cloakquest3r

# install kiterunner (Go + Node)
if ! command -v kiterunner >/dev/null; then
    git clone https://github.com/assetnote/kiterunner /tmp/kiterunner
    cd /tmp/kiterunner
    make build
    $SUDO cp ./dist/kr /usr/local/bin/kiterunner
    cd - && rm -rf /tmp/kiterunner
fi

# install gospider (Go)
if ! command -v gospider >/dev/null; then
    go install github.com/jaeles-project/gospider@latest
fi

# install XSStrike (pip)
pip3 install XSStrike

# install smuggler (Python script)
if ! command -v smuggler >/dev/null; then
    wget https://raw.githubusercontent.com/defparam/smuggler/master/smuggler.py -O /usr/local/bin/smuggler
    chmod +x /usr/local/bin/smuggler
fi

# install sniper/sn1per (git clone)
if ! command -v sniper >/dev/null; then
    git clone https://github.com/1N3/Sn1per.git /opt/Sn1per
    cd /opt/Sn1per
    bash install.sh || true
    cd -
fi

# install aquatone (Go)
go install github.com/michenriksen/aquatone@latest

# install gowitness (Go)
go install github.com/sensepost/gowitness@latest

# install whatweb (Ruby)
if ! command -v whatweb >/dev/null; then
    $SUDO $PKG install -y ruby ruby-dev || true
    $SUDO gem install whatweb
fi

# --- Nmap (via system pkg) ---
if ! command -v nmap >/dev/null; then
    $SUDO $PKG install -y nmap
fi

# --- Other utilities ---
if ! command -v jq >/dev/null; then
    $SUDO $PKG install -y jq
fi

echo -e "${GREEN}[+] All tools and BB-Runner installed! Add ~/go/bin to your PATH if not present.${NC}"
echo -e "${YELLOW}[!] If you want wordlists (for ffuf/gobuster/etc), run: git clone https://github.com/danielmiessler/SecLists.git ~/SecLists${NC}"
echo -e "${GREEN}[✓] Run ./bb-runner to start the TUI.${NC}"

