# Termaid Visual Workflow Editor Specification

## Overview

The Termaid Visual Editor is a web-based drag-and-drop interface for creating, editing, and visualizing bug bounty automation workflows. It provides an intuitive graphical interface that translates to the matrix-based JSON workflow format.

## Architecture

### Frontend
- **Technology**: React 18 + TypeScript
- **UI Framework**: Tailwind CSS + Headless UI
- **Diagramming**: React Flow / D3.js hybrid
- **State Management**: Zustand
- **Build Tool**: Vite

### Backend
- **API Server**: Go Fiber (HTTP REST API)
- **Workflow Engine**: Enhanced Termaid pipeline
- **File Storage**: Local filesystem + SQLite metadata
- **WebSocket**: Real-time execution updates

### Data Flow
```
[Visual Editor] <---> [Go API] <---> [Termaid Engine] <---> [File System]
     ^                    ^              ^                    ^
     |                    |              |                    |
   React UI         REST/WebSocket   Matrix Engine        Workflows
```

## User Interface Design

### Main Layout (Responsive)
```
┌─────────────────────────────────────────────────────────────────────────────┐
│ Header: [Logo] [Project] [Save] [Load] [Run] [Export] [Settings] [User]     │
├────────────┬────────────────────────────────────────────────────┬───────────┤
│  Toolbox   │                Canvas Area                         │Properties │
│            │                                                    │   Panel   │
│ ┌────────┐ │ ┌─────┐    ┌─────┐    ┌─────┐    ┌─────┐         │           │
│ │DNS     │ │ │Start│───▶│Tool1│───▶│Tool2│───▶│End  │         │ ┌───────┐ │
│ │Recon   │ │ └─────┘    └─────┘    └─────┘    └─────┘         │ │Node   │ │
│ └────────┘ │                                                    │ │Props  │ │
│ ┌────────┐ │     ┌─────┐                                       │ │       │ │
│ │Port    │ │     │Tool3│                                       │ │Name:  │ │
│ │Scan    │ │     └─────┘                                       │ │[____] │ │
│ └────────┘ │        │                                          │ │       │ │
│ ┌────────┐ │        ▼                                          │ │Args:  │ │
│ │Web     │ │     ┌─────┐    ┌─────┐                            │ │[____] │ │
│ │Apps    │ │     │Tool4│───▶│Tool5│                            │ │       │ │
│ └────────┘ │     └─────┘    └─────┘                            │ │Layer: │ │
│ ┌────────┐ │                                                    │ │  [2]  │ │
│ │Vulns   │ │                                                    │ │       │ │
│ │Scan    │ │                                                    │ └───────┘ │
│ └────────┘ │                                                    │           │
├────────────┼────────────────────────────────────────────────────┼───────────┤
│ Status Bar │ Matrix: 5x3 | Nodes: 12 | Parallel: 4 | Ready    │ [Help]    │
└────────────┴────────────────────────────────────────────────────┴───────────┘
```

### Canvas Features

#### Grid System
- **Matrix Overlay**: Visual X/Y coordinate grid
- **Layer Columns**: Vertical columns representing execution layers
- **Position Rows**: Horizontal rows for parallel positioning
- **Snap-to-Grid**: Automatic alignment to matrix coordinates

#### Node Representation
```
┌─────────────────┐
│ 🔧 subfinder    │ ← Tool icon + name
├─────────────────┤
│ Args: -d {{...}}│ ← Truncated arguments
├─────────────────┤
│ ⚡ Parallel: ON │ ← Execution mode
├─────────────────┤
│ [2,1] Layer 2   │ ← Matrix position
└─────────────────┘
```

#### Connection Types
- **Sequential Flow**: Solid arrows (→)
- **Parallel Branch**: Dashed arrows (⇢)
- **Conditional**: Diamond-shaped connectors
- **Data Merge**: Multiple inputs to single output

#### Visual States
- **Idle**: Gray border, white background
- **Selected**: Blue border, light blue background
- **Running**: Orange border, animated pulse
- **Complete**: Green border, checkmark icon
- **Error**: Red border, error icon
- **Disabled**: Grayed out, semi-transparent

### Toolbox Categories

#### DNS & Subdomain Discovery
```
┌─────────────────┐
│ 🌐 DNS Recon    │
├─────────────────┤
│ • subfinder     │
│ • assetfinder   │
│ • amass         │
│ • chaos-client  │
│ • dnsx          │
│ • massdns       │
└─────────────────┘
```

#### Port & Service Scanning
```
┌─────────────────┐
│ 🔍 Port Scan    │
├─────────────────┤
│ • naabu         │
│ • masscan       │
│ • nmap          │
│ • rustscan      │
└─────────────────┘
```

#### Web Application Testing
```
┌─────────────────┐
│ 🕷️ Web Apps     │
├─────────────────┤
│ • httpx         │
│ • ffuf          │
│ • gobuster      │
│ • feroxbuster   │
│ • katana        │
│ • nuclei        │
└─────────────────┘
```

#### Vulnerability Assessment
```
┌─────────────────┐
│ ⚠️ Vulns        │
├─────────────────┤
│ • sqlmap        │
│ • dalfox        │
│ • xsstrike      │
│ • arjun         │
│ • commix        │
└─────────────────┘
```

### Properties Panel

#### Node Configuration
```
┌─────────────────────────┐
│ Node Properties         │
├─────────────────────────┤
│ Name: [subfinder-1    ] │
│ Tool: [subfinder  ▼]  │
│                         │
│ Arguments:              │
│ ┌─────────────────────┐ │
│ │-d {{domain}}        │ │
│ │-silent              │ │
│ │-o {{output}}        │ │
│ └─────────────────────┘ │
│                         │
│ Matrix Position:        │
│ Layer (X): [2] Position │
│ (Y): [1]                │
│                         │
│ ☑️ Parallel Execution   │
│ ☐ Skip on Error        │
│ ☐ Required for Success │
│                         │
│ Timeout: [300] seconds  │
│ Retries: [1]            │
│                         │
│ [Apply] [Reset]         │
└─────────────────────────┘
```

#### Workflow Settings
```
┌─────────────────────────┐
│ Workflow Settings       │
├─────────────────────────┤
│ Name: [Web App Scan   ] │
│ Description:            │
│ ┌─────────────────────┐ │
│ │Comprehensive web    │ │
│ │application security │ │
│ │assessment...        │ │
│ └─────────────────────┘ │
│                         │
│ Target Domain:          │
│ [example.com        ]   │
│                         │
│ Concurrency: [6]        │
│ Global Timeout: [3600]s │
│                         │
│ Output Format:          │
│ ☑️ JSON  ☑️ TXT  ☐ XML  │
│                         │
│ [Save] [Load] [Export]  │
└─────────────────────────┘
```

## Core Features

### Drag & Drop Operations

#### Tool Placement
1. **Drag from Toolbox**: Click and drag tool to canvas
2. **Auto-positioning**: Snap to next available matrix position
3. **Visual Feedback**: Highlight valid drop zones
4. **Connection Preview**: Show potential connections while dragging

#### Node Manipulation
1. **Move Nodes**: Drag to reposition in matrix
2. **Connect Nodes**: Drag from output port to input port
3. **Disconnect**: Right-click connection to remove
4. **Multi-select**: Ctrl+click or drag-select multiple nodes

#### Connection Logic
- **Automatic Routing**: Smart path finding between nodes
- **Collision Avoidance**: Connections avoid overlapping nodes
- **Port Validation**: Prevent invalid connections
- **Bidirectional Flow**: Visual indicators for data flow direction

### Real-time Collaboration

#### Multi-user Editing
- **User Cursors**: Show other users' mouse positions
- **Live Updates**: Real-time workflow synchronization
- **Conflict Resolution**: Last-write-wins with visual notifications
- **User Presence**: Display active collaborators

#### Version Control
- **Auto-save**: Continuous workflow saving
- **History**: Undo/redo stack with branching
- **Snapshots**: Named versions for major changes
- **Diff Viewer**: Visual comparison between versions

### Execution Monitoring

#### Live Execution View
```
┌─────────────────────────────────────────────────────────────┐
│ Execution Monitor                               [⏸️] [⏹️]    │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ ┌─────┐ ✅  ┌─────┐ 🔄  ┌─────┐ ⏳  ┌─────┐ ⏸️           │
│ │Start│────▶│Tool1│────▶│Tool2│────▶│Tool3│               │
│ └─────┘     └─────┘     └─────┘     └─────┘               │
│   Done       Running     Queue      Waiting               │
│                                                             │
│             ┌─────┐ ❌                                     │
│             │Tool4│                                        │
│             └─────┘                                        │
│              Error                                         │
│                                                             │
├─────────────────────────────────────────────────────────────┤
│ Progress: ████████████████████░░░░ 80% (4/5 complete)      │
│ Runtime: 00:15:23 | Est. Remaining: 00:03:45               │
└─────────────────────────────────────────────────────────────┘
```

#### Result Integration
- **Live Output**: Stream tool outputs to UI
- **Progress Tracking**: Real-time completion percentages
- **Error Handling**: Visual error states with logs
- **Result Preview**: Quick preview of tool outputs

### Template Management

#### Template Library
```
┌─────────────────────────────────────────────────────────────┐
│ Template Library                                    [➕ New] │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│ 🎯 Bug Bounty Templates                                     │
│ ├─ Quick Subdomain Scan (15 min) ⭐⭐⭐⭐⭐ [Load] [Edit]   │
│ ├─ Comprehensive Web App (3-6 hrs) ⭐⭐⭐⭐⭐ [Load] [Edit] │
│ ├─ API Security Assessment (1-3 hrs) ⭐⭐⭐⭐☐ [Load] [Edit]│
│ └─ Mobile Backend Testing (1-2 hrs) ⭐⭐⭐☐☐ [Load] [Edit]  │
│                                                             │
│ 🏢 Enterprise Templates                                     │
│ ├─ Active Directory Enum (30-90 min) ⭐⭐⭐⭐☐ [Load] [Edit]│
│ ├─ Internal Network Scan (30-60 min) ⭐⭐⭐⭐⭐ [Load] [Edit]│
│ └─ Cloud Asset Discovery (1-2 hrs) ⭐⭐⭐☐☐ [Load] [Edit]   │
│                                                             │
│ 🚀 Custom Templates                                         │
│ ├─ My Web App Template ⭐⭐⭐⭐☐ [Load] [Edit] [Delete]      │
│ └─ Client XYZ Workflow ⭐⭐⭐☐☐ [Load] [Edit] [Delete]       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
```

#### Template Features
- **Rating System**: Community ratings for template quality
- **Usage Statistics**: Track template popularity
- **Customization**: Fork and modify existing templates
- **Sharing**: Export/import templates with team

### Advanced Features

#### Conditional Logic
```
┌─────────────────────────────────────────────────────────────┐
│                    ┌─────┐                                  │
│              ┌────▶│Tool2│                                  │
│   ┌─────┐    │     └─────┘                                  │
│   │Tool1│────┤                                              │
│   └─────┘    │     ┌─────┐                                  │
│              └────▶│Tool3│                                  │
│                    └─────┘                                  │
│                                                             │
│ Condition: if(http_status == 200) → Tool2                  │
│           else → Tool3                                      │
└─────────────────────────────────────────────────────────────┘
```

#### Subgraph Management
- **Group Creation**: Select multiple nodes to create subgraph
- **Parallel Execution**: Configure entire subgraphs for parallel execution
- **Nested Workflows**: Subgraphs can contain other subgraphs
- **Resource Allocation**: Set concurrency limits per subgraph

#### Performance Optimization
- **Resource Monitoring**: Real-time CPU/memory usage
- **Auto-scaling**: Dynamic concurrency adjustment
- **Bottleneck Detection**: Identify slow workflow components
- **Optimization Suggestions**: AI-powered workflow improvements

## Technical Implementation

### API Endpoints

#### Workflow Management
```
GET    /api/workflows          # List all workflows
POST   /api/workflows          # Create new workflow
GET    /api/workflows/{id}     # Get workflow details
PUT    /api/workflows/{id}     # Update workflow
DELETE /api/workflows/{id}     # Delete workflow
POST   /api/workflows/{id}/run # Execute workflow
```

#### Real-time Updates
```
WebSocket /ws/workflow/{id}    # Live workflow updates
WebSocket /ws/execution/{id}   # Execution monitoring
```

#### Template Operations
```
GET    /api/templates          # List templates
POST   /api/templates          # Create template
GET    /api/templates/{id}     # Get template
PUT    /api/templates/{id}     # Update template
POST   /api/templates/{id}/fork # Fork template
```

### Data Models

#### Workflow Schema
```typescript
interface Workflow {
  id: string;
  name: string;
  description: string;
  version: string;
  matrix: {
    max_x: number;
    max_y: number;
  };
  subgraphs: Subgraph[];
  nodes: Node[];
  metadata: WorkflowMetadata;
}

interface Node {
  id: string;
  tool: string;
  args: string;
  children: string[];
  layer: number;
  position: number;
  parallel: boolean;
  subgraph?: string;
  sub_x?: number;
  sub_y?: number;
  ui_position: {
    x: number;
    y: number;
  };
}
```

#### Execution State
```typescript
interface ExecutionState {
  workflow_id: string;
  run_id: string;
  status: 'pending' | 'running' | 'completed' | 'failed';
  start_time: Date;
  end_time?: Date;
  nodes: {
    [nodeId: string]: NodeExecutionState;
  };
  statistics: ExecutionStatistics;
}

interface NodeExecutionState {
  status: 'pending' | 'running' | 'completed' | 'failed' | 'skipped';
  start_time?: Date;
  end_time?: Date;
  output_files: string[];
  error_message?: string;
  progress?: number;
}
```

### Frontend Components

#### Canvas Component
```typescript
interface CanvasProps {
  workflow: Workflow;
  selectedNodes: string[];
  executionState?: ExecutionState;
  onNodeSelect: (nodeIds: string[]) => void;
  onNodeMove: (nodeId: string, position: Position) => void;
  onConnection: (source: string, target: string) => void;
  onDisconnection: (connectionId: string) => void;
}
```

#### Node Component
```typescript
interface NodeComponentProps {
  node: Node;
  isSelected: boolean;
  executionState?: NodeExecutionState;
  onSelect: () => void;
  onMove: (position: Position) => void;
  onEdit: () => void;
  onDelete: () => void;
}
```

### Security Considerations

#### Authentication & Authorization
- **OAuth 2.0**: Integration with GitHub/Google/enterprise SSO
- **Role-based Access**: Read/write/admin permissions per workflow
- **API Keys**: Secure access for programmatic usage
- **Audit Logging**: Track all workflow modifications

#### Execution Security
- **Sandboxing**: Isolated execution environments
- **Resource Limits**: CPU/memory/time constraints
- **Input Validation**: Sanitize all tool arguments
- **Output Filtering**: Remove sensitive data from outputs

## Development Phases

### Phase 1: Core Editor (4-6 weeks)
- Basic drag-and-drop functionality
- Node creation and connection
- Properties panel
- Save/load workflows
- Matrix positioning system

### Phase 2: Execution Integration (3-4 weeks)
- Workflow execution via API
- Real-time status updates
- Basic result viewing
- Error handling and recovery

### Phase 3: Advanced Features (4-5 weeks)
- Template management system
- Collaborative editing
- Advanced node configurations
- Performance monitoring

### Phase 4: Polish & Optimization (2-3 weeks)
- UI/UX improvements
- Performance optimization
- Documentation
- Testing and bug fixes

## Success Metrics

### User Experience
- **Time to Create Workflow**: < 5 minutes for simple workflows
- **Learning Curve**: New users productive within 30 minutes
- **Error Rate**: < 5% of workflows fail due to UI issues
- **User Satisfaction**: > 4.5/5 rating

### Technical Performance
- **Load Time**: < 3 seconds for complex workflows
- **Responsiveness**: < 100ms for UI interactions
- **Scalability**: Support 50+ concurrent users
- **Reliability**: 99.9% uptime

### Business Impact
- **Adoption Rate**: 80% of users prefer visual editor
- **Workflow Quality**: 50% reduction in workflow errors
- **Productivity**: 3x faster workflow creation
- **Community Growth**: 2x increase in shared templates

## Future Enhancements

### AI-Powered Features
- **Smart Suggestions**: AI-recommended next tools
- **Auto-optimization**: Performance improvement suggestions
- **Anomaly Detection**: Identify unusual execution patterns
- **Natural Language**: "Create a web app scan workflow"

### Advanced Visualizations
- **3D Matrix View**: Three-dimensional workflow representation
- **Timeline View**: Temporal execution visualization
- **Dependency Graph**: Complex relationship mapping
- **Performance Heatmap**: Resource usage visualization

### Integration Ecosystem
- **Plugin System**: Third-party tool integration
- **API Marketplace**: Community-contributed connectors
- **Cloud Providers**: AWS/Azure/GCP native integration
- **CI/CD Pipelines**: GitHub Actions/Jenkins integration