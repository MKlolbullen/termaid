# Termaid

**Termaid is a term (pun intended) for Terminal-Mermaid. I started fooling around with mermaid charts and realized that they're a super convient way to create automation templates (using https://mermaid.live - it's basically drag-n-drop + AI. Termaid is that and so, so much more. I've tried to create a trickest-like platform, using https://charm.sh libraries to make it look, feel and act fantastic.**

**Termaid**: a next-level, terminal-native automation framework for bug bounty hunters and penetration testers. 🌐🎯
Build workflows, execute recon, and chain your favorite tools — all from a sleek, interactive TUI.
No mouse, no fuss — just pure hacker speed.
A terminal-based bug bounty hunting automation tool with visual workflow management using Mermaid charts. Build, visualize, and execute complex reconnaissance pipelines entirely from your terminal.

## Credits
Since this platform is a work in progress and was greatly inspired by trickest.io, a web-based platform that does this, only not hosted locally but in their much fancier web interface, but not everyone can afford the subscription plan, or perhaps hosting locally is preferred by some? Either way, Thank you Trickest, projectdiscovery, obviously mermaid charts and A LOT of insanely talented hackers, crackers and subjackers... And some developers as well, check out my stars/follows for some of the ones I really appreciate.

## Features

- **Visual Workflow Builder**: Create complex bug bounty workflows using an interactive TUI
- **Mermaid Integration**: Visualize your workflows as directed acyclic graphs (DAGs)
- **Pre-built Tool Catalog**: 60+ popular bug bounty tools with default configurations
- **Template System**: Save and reuse workflow templates
- **Parallel Execution**: Run tools concurrently with configurable concurrency limits
- **Real-time Monitoring**: Live status updates and logging during execution
- **Pipeline Management**: Automatic data flow between tools with deduplication
- **Extensible**: Easy to add new tools and customize existing ones

## Installation

### Prerequisites

- Go 1.22+ 
- Python 3.x
- Node.js (for some tools)
- Common bug bounty tools (subfinder, httpx, nuclei, etc.)

### Quick Install

```bash
git clone https://github.com/MKlolbullen/termaid.git
cd termaid
chmod +x install.sh
./install.sh
```

The installer will:
- Install Go if missing
- Install Python and Node.js dependencies 
- Install popular bug bounty tools
- Build the termaid binary

### Manual Build

```bash
go mod tidy
go build -o termaid ./cmd/termaid
```

## Usage

Launch the TUI:

```bash
./termaid
```

### Main Menu Options

1. **Run Workflow** - Execute the default workflow.json
2. **Run Template** - Choose from saved workflow templates
3. **Preview Workflow** - View Mermaid diagram of current workflow
4. **Create Workflow** - Open the visual workflow builder
5. **Exit** - Quit the application

## Workflow Builder

The interactive workflow builder allows you to:

### Navigation
- `Tab` / `Shift+Tab` - Cycle between panels
- `↑/↓` - Navigate within panels or layers
- `Enter` - Select items

### Building Workflows
- `n` - Add selected tool to workflow (when in Tools panel)
- `r` - Remove selected node (when in Canvas panel)
- `c` - Commit/save arguments (when in Args panel)
- `f` - Finish and save workflow

### Panels
1. **Domain Input** - Target domain for the workflow
2. **Tools List** - Available tools from catalog
3. **Canvas** - Visual representation of workflow layers
4. **Args Editor** - Modify tool arguments

## Tool Catalog

Termaid includes 60+ pre-configured tools organized by category:

### DNS / Subdomain Discovery
- subfinder, assetfinder, amass, chaos-client
- dnsx, jsubfinder, massdns, bbot
- subjack, subover, subzy, uncover

### Port Scanning / Probing  
- httpx, httprobe, naabu, nmap, smuggler

### URL Gathering / Crawling
- gau, gauplus, gospider, katana
- waymore, urx, urlfinder, Photon

### Parameter Discovery
- arjun, JSFinder, Linkfinder, oralyzer
- parameth, paramspider, unfurl

### Fuzzing / Content Discovery
- ffuf, gobuster, cariddi, kiterunner
- cloakquest3r, Corsy

### Vulnerability Scanning
- nuclei, dalfox, sqlmap, XSStrike
- crlfuzz, kxss, SSRFire, tsunami
- wafw00f, xsrfprobe, scan4all

### Utilities
- aquatone, gowitness, whatweb, gf
- puredns, trufflehog

## Workflow Format

Workflows are JSON files with the following structure:

```json
{
  "workflow": [
    {
      "id": "subfinder-1",
      "tool": "subfinder", 
      "args": "-d {{domain}} -silent -o {{output}}",
      "children": ["httpx-1"],
      "layer": 1
    },
    {
      "id": "httpx-1",
      "tool": "httpx",
      "args": "-l {{input}} -title -tech-detect -json -o {{output}}",
      "children": ["nuclei-1"], 
      "layer": 2
    },
    {
      "id": "nuclei-1",
      "tool": "nuclei",
      "args": "-l {{input}} -severity medium,high,critical -o {{output}}",
      "children": [],
      "layer": 3
    }
  ]
}
```

### Placeholders

- `{{domain}}` - Target domain
- `{{input}}` - Input file from previous layer
- `{{output}}` - Output file for current tool

## Examples

### Basic Subdomain Enumeration

```bash
# Create workflow.json with subfinder -> httpx -> nuclei
# Run with domain
./termaid
# Select "Run Workflow", enter target domain
```

### Custom Workflow

1. Launch termaid
2. Select "Create Workflow"  
3. Enter target domain
4. Select tools from catalog
5. Press `n` to add tools
6. Modify arguments with `c`
7. Save with `f`

### Template Usage

```bash
# Save workflows in ./workflows/ directory
# Select "Run Template" to choose existing workflows
```

## Directory Structure

```
termaid/
├── assets/
│   └── tools.yaml          # Tool catalog definitions
├── workflows/              # Saved workflow templates
├── internal/
│   ├── tui/               # Terminal UI components
│   ├── graph/             # DAG management
│   └── pipeline/          # Execution engine
├── workdir/               # Runtime output directory
└── cmd/termaid/           # Main application
```

## Configuration

### Adding New Tools

Edit `assets/tools.yaml`:

```yaml
- name: mytool
  cat: Custom
  desc: My custom tool
  def: "-flag {{domain}} -o {{output}}"
```

### Workflow Templates

Save JSON workflows in the `workflows/` directory. They'll appear in the "Run Template" menu.

## Output

- Execution logs: `run-<timestamp>.log`
- Tool outputs: `workdir/<category>/<tool>_<id>.txt`
- Merged results: `workdir/<category>/merged.txt`

## Keyboard Shortcuts

### Main Menu
- `q` / `Ctrl+C` - Quit
- `Enter` - Select option

### Workflow Execution
- `Tab` - Toggle log view
- `q` - Quit execution
- `↑/↓` - Scroll logs (when log view active)

### Workflow Builder
- `Tab` / `Shift+Tab` - Switch panels
- `n` - Add node
- `r` - Remove node  
- `c` - Commit args
- `f` - Finish/save
- `↑/↓` - Navigate

## Requirements

### System Dependencies
- curl, wget, git, jq, unzip
- Build tools (gcc, make)

### Go Tools (auto-installed)
- subfinder, httpx, nuclei, naabu
- dnsx, chaos-client, amass, ffuf
- gau, gauplus, dalfox, kxss

### Python Tools (auto-installed)
- arjun, xsrfprobe, waymore
- parameth, photon

### Optional Tools
- glow (for Mermaid preview)
- Various external tools per your needs

## Troubleshooting

### Command Not Found Errors
Ensure tools are installed and in PATH:
```bash
which subfinder httpx nuclei
```

### Permission Errors
Check directory permissions:
```bash
chmod 755 workdir workflows
```

### Missing Dependencies
Re-run installer:
```bash
./install.sh
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tools to `assets/tools.yaml`
4. Update documentation
5. Submit a pull request

## License

MIT License - see LICENSE file for details.

## Acknowledgments

- Built with [Charm.sh](https://charm.sh) TUI libraries
- Integrates popular bug bounty tools from the community
- Inspired by visual workflow tools and automation frameworks
