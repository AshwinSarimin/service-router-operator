# Service Router Operator - Architecture Diagrams

This directory contains comprehensive architecture diagrams for the Service Router Operator, created using Mermaid diagram syntax.

## 📊 Available Diagrams

### 1. [architecture.mermaid](architecture.mermaid)
**Complete System Architecture**

Shows the full system architecture including:
- All Custom Resource Definitions (CRDs) - both cluster-scoped and namespace-scoped
- Operator controllers and internal cache
- Generated resources (Istio Gateway, VirtualService, DNSEndpoints)
- External integrations (ExternalDNS, Azure Private DNS, Load Balancer)
- User roles and their interactions (Platform Team, Application Team, End Users)
- Complete data flow from CRD creation to client request handling

**Best for**: Understanding the complete system at a glance, onboarding new team members

---

### 2. [reconciliation-flow.mermaid](reconciliation-flow.mermaid)
**Reconciliation Flow Sequence Diagram**

Detailed sequence diagram showing:
- Step-by-step reconciliation process
- Controller interactions and watch events
- Cache operations and data flow
- Resource generation and updates
- Complete flow from CRD creation to DNS resolution
- Client request routing through the system
- Update flow when resources change

**Best for**: Understanding how the operator works internally, troubleshooting reconciliation issues

---

### 3. [dns-routing-modes.mermaid](dns-routing-modes.mermaid)
**DNS Routing Modes Comparison**

Compares three DNS routing scenarios:
1. **Active Mode**: Regional isolation - only creates DNS records in the current region
2. **RegionBound Mode**: Cross-region consolidation - creates DNS records in all regions pointing to one cluster
3. **Migration Scenario**: Orphan region adoption - allows a cluster to adopt regions without active clusters

Each mode includes:
- Logic for controller selection
- DNS records created in each region
- Traffic routing results
- Use case recommendations

**Best for**: Understanding when to use each mode, planning multi-region deployments

---

### 4. [crd-relationships.mermaid](crd-relationships.mermaid)
**CRD Relationships and Ownership**

Comprehensive view of:
- All CRDs with their ownership (Platform vs Application Team)
- Scope (cluster-scoped vs namespace-scoped)
- Cardinality rules (singleton vs multiple instances)
- Resource generation relationships (ownerReferences)
- Read dependencies between resources
- Reconciliation order
- Deletion cascade behavior

**Best for**: Understanding resource relationships, planning RBAC policies, understanding lifecycle management

---

## 🎨 Viewing the Diagrams

### In VS Code
1. Install the [Markdown Preview Mermaid Support](https://marketplace.visualstudio.com/items?itemName=bierner.markdown-mermaid) extension
2. Open any `.mermaid` file
3. Right-click and select "Open Preview" or use `Ctrl+Shift+V`

### In GitHub
GitHub natively supports Mermaid diagrams in Markdown files. The diagrams will render automatically when viewing this README or any Markdown file containing Mermaid code blocks.

### Online Editors
- [Mermaid Live Editor](https://mermaid.live/) - Official online editor
- Copy the diagram code and paste it into the editor for interactive viewing and editing

### In Documentation
To embed these diagrams in Markdown documentation:

```markdown
```mermaid
{paste diagram code here}
```
```

## 📚 Related Documentation

- [ARCHITECTURE.md](../docs/ARCHITECTURE.md) - Detailed architecture documentation
- [USER-GUIDE.md](../docs/USER-GUIDE.md) - User guide with examples
- [OPERATOR-GUIDE.md](../docs/OPERATOR-GUIDE.md) - Operator deployment and configuration
- [DEVELOPMENT.md](../docs/DEVELOPMENT.md) - Development guide for contributors

## 🔄 Updating Diagrams

When updating diagrams:
1. Follow the [Mermaid Instructions](../.github/instructions/mermaid.instructions.md)
2. Use consistent styling and color schemes (see existing diagrams)
3. Test rendering in VS Code before committing
4. Update this README if adding new diagrams
5. Keep diagrams in sync with code changes

## 🎯 Diagram Guidelines

Based on the project's Mermaid instructions:
- Use `<br/>` for line breaks in text (not `\n`)
- Use consistent participant naming in sequence diagrams
- Use `box` blocks to visually group related components
- Use `Note` for clarifying steps or adding context
- Keep descriptions concise and clear
- Use proper indentation for readability
- Include comments for complex flows

## 💡 Tips for Understanding

1. **Start with architecture.mermaid** for the big picture
2. **Move to reconciliation-flow.mermaid** to understand the dynamics
3. **Use dns-routing-modes.mermaid** when planning your deployment strategy
4. **Reference crd-relationships.mermaid** when writing custom resources

## 🤝 Contributing

When contributing new diagrams:
1. Follow existing naming conventions (`kebab-case.mermaid`)
2. Add comprehensive comments in the diagram source
3. Update this README with diagram description
4. Ensure diagrams follow the project's Mermaid instructions
5. Test rendering in multiple environments

## 📝 Notes

- All diagrams use Mermaid syntax for maximum compatibility
- Diagrams are version-controlled alongside code for traceability
- Color schemes follow consistent patterns for resource types
- Icons and emojis used for visual clarity (👥 🌐 📦 ⚙️ etc.)
