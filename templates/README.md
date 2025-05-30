# Termaid Professional Template Library

## Overview

A curated collection of advanced workflow templates designed for experienced bug bounty hunters, penetration testers, and security researchers. Each template represents battle-tested methodologies used in real-world engagements.

## Template Categories

### 🎯 **Reconnaissance Templates**

**`external-recon-comprehensive.json`** - Complete external reconnaissance
- Subdomain enumeration (passive + active)
- DNS analysis and zone transfers
- Port scanning and service fingerprinting
- Web technology stack identification
- SSL/TLS analysis and certificate transparency
- Social media and OSINT gathering
- **Estimated Runtime**: 2-4 hours
- **Concurrency**: High (15+ parallel processes)

**`internal-network-discovery.json`** - Internal network reconnaissance
- Network range discovery
- Live host detection
- Service enumeration
- SMB/NetBIOS scanning
- LDAP enumeration
- **Use Case**: Post-exploitation or internal pentest
- **Estimated Runtime**: 30-60 minutes

**`cloud-asset-discovery.json`** - Cloud infrastructure reconnaissance
- AWS/Azure/GCP subdomain patterns
- S3 bucket enumeration
- Cloud service fingerprinting
- Certificate transparency mining
- **Use Case**: Cloud-focused engagements
- **Estimated Runtime**: 1-2 hours

### 🕷️ **Web Application Security Templates**

**`webapp-comprehensive-scan.json`** - Full web application assessment
- Content discovery (directories, files, parameters)
- Authentication bypass testing
- Input validation and injection testing
- Session management analysis
- CORS and security header analysis
- **Target**: Modern web applications
- **Estimated Runtime**: 3-6 hours

**`api-security-assessment.json`** - REST/GraphQL API testing
- Endpoint discovery via fuzzing
- Parameter pollution testing
- Rate limiting bypass
- Authentication token analysis
- GraphQL introspection and injection
- **Target**: API endpoints and microservices
- **Estimated Runtime**: 1-3 hours

**`js-analysis-deep.json`** - JavaScript security analysis
- Source map discovery
- Hardcoded credential extraction
- DOM XSS analysis
- Prototype pollution testing
- Webpack/bundler analysis
- **Target**: Single-page applications
- **Estimated Runtime**: 1-2 hours

### 🔓 **Vulnerability-Focused Templates**

**`sqli-comprehensive.json`** - SQL injection testing
- Error-based injection detection
- Blind boolean-based testing
- Time-based blind injection
- Union-based data extraction
- NoSQL injection patterns
- **Focus**: Database interaction points
- **Estimated Runtime**: 2-4 hours

**`xss-comprehensive.json`** - Cross-site scripting assessment
- Reflected XSS detection
- Stored XSS hunting
- DOM-based XSS analysis
- CSP bypass techniques
- Filter evasion testing
- **Focus**: Input fields and output contexts
- **Estimated Runtime**: 2-3 hours

**`ssrf-and-rce.json`** - Server-side request forgery & RCE
- SSRF detection and exploitation
- Command injection testing
- File upload bypass techniques
- Deserialization vulnerability testing
- Template injection analysis
- **Focus**: High-impact vulnerabilities
- **Estimated Runtime**: 1-2 hours

### 🏢 **Enterprise & Advanced Templates**

**`active-directory-enum.json`** - Active Directory enumeration
- Domain controller discovery
- User and group enumeration
- Kerberos ticket analysis
- LDAP query optimization
- BloodHound data collection
- **Use Case**: Windows domain environments
- **Estimated Runtime**: 30-90 minutes

**`mobile-app-backend.json`** - Mobile application backend testing
- API endpoint discovery
- Certificate pinning bypass detection
- Deep link enumeration
- Push notification testing
- Backend service fingerprinting
- **Target**: Mobile app infrastructure
- **Estimated Runtime**: 1-2 hours

**`iot-device-assessment.json`** - IoT device security testing
- Firmware analysis preparation
- Network service enumeration
- Default credential testing
- MQTT/CoAP protocol testing
- Update mechanism analysis
- **Target**: Connected devices
- **Estimated Runtime**: 1-3 hours

### ⚡ **Quick Assessment Templates**

**`subdomain-fast.json`** - Rapid subdomain discovery (15 minutes)
**`port-scan-top1000.json`** - Quick port scanning (10 minutes)
**`web-quick-wins.json`** - Common web vulnerabilities (30 minutes)
**`ssl-tls-analysis.json`** - SSL/TLS configuration testing (5 minutes)

### 🎯 **Specialized Methodology Templates**

**`bug-bounty-automation.json`** - Automated bug bounty workflow
- Optimized for continuous scanning
- Rate-limited requests
- Output formatting for triage
- False positive reduction
- **Target**: Bug bounty programs
- **Estimated Runtime**: 1-4 hours

**`red-team-stealth.json`** - Low-noise reconnaissance
- Passive techniques only
- Minimal network footprint
- OSINT-heavy approach
- **Use Case**: Red team engagements
- **Estimated Runtime**: 2-6 hours

**`compliance-audit.json`** - Security compliance assessment
- OWASP Top 10 coverage
- Security header validation
- SSL/TLS compliance checking
- **Use Case**: Compliance audits
- **Estimated Runtime**: 1-2 hours

## Template Complexity Levels

### **Beginner** (🟢)
- Linear workflows
- Sequential execution
- Basic tool combinations
- Limited parallelization

### **Intermediate** (🟡)
- Branching workflows
- Moderate parallelization
- Tool result correlation
- Conditional execution paths

### **Advanced** (🔴)
- Complex matrix positioning
- Heavy parallelization
- Multi-stage analysis
- Advanced result processing

### **Expert** (⚫)
- Highly optimized workflows
- Custom tool integration
- Dynamic adaptation
- Performance-tuned execution

## Usage Instructions

### Quick Start
```bash
# Copy template to active workflow
cp templates/webapp-comprehensive-scan.json workflow.json

# Launch Termaid and select "Run Workflow"
./termaid
```

### Customization
1. **Modify target scope**: Update domain placeholders
2. **Adjust concurrency**: Tune parallel execution limits
3. **Add custom tools**: Extend with organization-specific tools
4. **Configure output**: Customize result formatting

### Best Practices

#### **Resource Management**
- Monitor system resources during heavy parallel execution
- Adjust concurrency based on target infrastructure
- Use rate limiting for sensitive environments

#### **Output Organization**
- Results automatically organized by workflow step
- Each tool creates timestamped output files
- Merged results available for cross-correlation

#### **Stealth Considerations**
- Choose appropriate templates for engagement rules
- Modify request timing for low-noise operations
- Remove aggressive tools for production environments

## Template Metadata

Each template includes:
- **Estimated runtime**
- **Resource requirements**
- **Target environment type**
- **Methodology reference**
- **Tool dependency list**
- **Output format description**

## Contributing Templates

### Submission Guidelines
1. **Real-world tested**: Templates must be validated in actual engagements
2. **Documentation**: Include methodology reasoning and expected outcomes
3. **Performance data**: Provide runtime estimates and resource usage
4. **Compliance**: Ensure templates follow responsible disclosure principles

### Template Structure
```json
{
  "metadata": {
    "name": "Template Name",
    "description": "Detailed description",
    "author": "Contributor",
    "version": "1.0",
    "methodology": "Reference framework",
    "estimated_runtime": "1-2 hours",
    "complexity": "Advanced",
    "target_type": "Web Application",
    "stealth_level": "Medium"
  },
  "workflow": [ /* nodes */ ]
}
```

## Integration with Security Frameworks

### **OWASP Integration**
- Templates mapped to OWASP Top 10
- ASVS compliance checking
- SAMM methodology alignment

### **NIST Cybersecurity Framework**
- Identify, Protect, Detect categories
- Risk assessment integration
- Compliance reporting support

### **PTES (Penetration Testing Execution Standard)**
- Pre-engagement phase templates
- Intelligence gathering workflows
- Vulnerability analysis methodologies

## Advanced Features

### **Conditional Execution**
- Templates can include conditional branches
- Results from previous steps influence subsequent execution
- Dynamic tool selection based on discovered services

### **Result Correlation**
- Cross-reference findings between tools
- Automated false positive reduction
- Vulnerability confidence scoring

### **Custom Integration**
- Hook points for custom scripts
- API integration capabilities
- Third-party tool embedding

## Professional Use Cases

### **Penetration Testing Firms**
- Standardized methodology enforcement
- Consistent reporting formats
- Quality assurance through templates

### **Bug Bounty Hunters**
- Proven attack patterns
- Optimized for speed and coverage
- Platform-specific adaptations

### **Enterprise Security Teams**
- Internal security assessments
- Continuous monitoring workflows
- Compliance validation

### **Security Researchers**
- Methodology experimentation
- Tool comparison frameworks
- Research reproducibility

## Support and Community

### **Template Updates**
- Regular updates for new tools and techniques
- Community-driven improvements
- Security advisory integration

### **Training Materials**
- Video walkthroughs for complex templates
- Methodology explanations
- Tool configuration guides

### **Professional Services**
- Custom template development
- Methodology consulting
- Training and certification programs

---

*This template library represents collective knowledge from the security community. Use responsibly and in accordance with applicable laws and authorized testing agreements.*