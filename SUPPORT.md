# Support

## Getting Help

We're here to help! If you need assistance with VKAI Panel, there are several ways to get support.

## Documentation

Before reaching out for help, please check our comprehensive documentation:

- [Panel Access Guide](docs/PANEL_ACCESS.md) - Panel port, security entrance, IP allow list, TLS.
  **Start here** if you cannot reach the panel: it does not answer on 80/443.
- [API Documentation](docs/API.md) - Complete API reference
- [User Guide](docs/USER_GUIDE.md) - End-user documentation
- [Developer Guide](docs/DEVELOPER_GUIDE.md) - Developer documentation
- [Configuration Guide](docs/CONFIGURATION.md) - Configuration options
- [Deployment Guide](docs/DEPLOYMENT.md) - Deployment instructions
- [Testing Guide](docs/TESTING.md) - Testing procedures
- [Security Guide](docs/SECURITY.md) - Security best practices
- [Contributing Guide](docs/CONTRIBUTING.md) - How to contribute
- [Troubleshooting Guide](docs/TROUBLESHOOTING.md) - Common issues and solutions
- [FAQ](docs/FAQ.md) - Frequently asked questions

## Community Support

### GitHub Discussions

For general questions, feature requests, and community discussions:

**GitHub Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions

**Categories**:
- **General**: General questions and discussions
- **Ideas**: Feature requests and suggestions
- **Q&A**: Questions and answers
- **Show and Tell**: Share your projects and setups

### GitHub Issues

For bug reports and technical issues:

**GitHub Issues**: https://github.com/hitechcloud-vietnam/vkai-panel/issues

**Before Creating an Issue**:
1. Search existing issues
2. Check the documentation
3. Try the troubleshooting guide

**When Creating an Issue**:
1. Use the issue template
2. Provide detailed information
3. Include steps to reproduce
4. Attach relevant logs

### Discord Community

Join our Discord community for real-time support:

**Discord**: https://discord.gg/vkai-panel

**Channels**:
- #general - General discussion
- #support - Technical support
- #development - Development discussion
- #announcements - Important announcements

### Stack Overflow

For technical questions:

**Stack Overflow**: https://stackoverflow.com/questions/tagged/vkai-panel

**Tags**:
- vkai-panel
- hosting-panel
- go
- nextjs

## Professional Support

### Enterprise Support

For enterprise customers, we offer professional support:

**Features**:
- Priority support
- Dedicated support engineer
- Phone support
- Custom development
- Training and consulting

**Contact**:
- **Email**: enterprise@hitechcloud.vn
- **Phone**: +84 xxx xxx xxx
- **Website**: https://hitechcloud.vn/enterprise

### Consulting Services

We offer consulting services for:

- Custom development
- System integration
- Performance optimization
- Security auditing
- Training and workshops

**Contact**:
- **Email**: consulting@hitechcloud.vn
- **Website**: https://hitechcloud.vn/consulting

## Bug Reports

### How to Report a Bug

1. **Check Existing Issues**: Search for similar issues
2. **Use the Template**: Use the bug report template
3. **Provide Details**: Include all relevant information
4. **Steps to Reproduce**: Clear steps to reproduce the issue
5. **Expected Behavior**: What you expected to happen
6. **Actual Behavior**: What actually happened
7. **Environment**: OS, architecture, panel version, affected service (`vkai-api` / `vkai-ui` / `vkai-agent`), browser
8. **Logs**: Relevant log entries
9. **Screenshots**: If applicable

### Bug Report Template

Opening an issue on GitHub loads
[`.github/ISSUE_TEMPLATE/bug_report.md`](.github/ISSUE_TEMPLATE/bug_report.md)
automatically. It asks for the bug description, reproduction steps, expected and
actual behaviour, and this environment block:

- Panel version, OS and CPU architecture
- How it was installed and the installation directory
- Which component is affected: `vkai-api`, `vkai-ui`, `vkai-agent` or the `vkai` CLI
- Browser, when the problem is in the interface
- Web server in use

Collect logs with:

```bash
sudo journalctl -u vkai-api -n 200 --no-pager
sudo journalctl -u vkai-ui -n 200 --no-pager
sudo tail -n 200 /vkai-panel/logs/*.log
```

Redact real IP addresses, domains, tokens and above all the panel's security
entrance path before pasting anything into a public issue.

## Feature Requests

### How to Request a Feature

1. **Check Existing Requests**: Search for similar requests
2. **Use the Template**: Use the feature request template
3. **Provide Details**: Include all relevant information
4. **Use Case**: Explain your use case
5. **Proposed Solution**: Suggest a solution (optional)

### Feature Request Template

Opening a feature request on GitHub loads
[`.github/ISSUE_TEMPLATE/feature_request.md`](.github/ISSUE_TEMPLATE/feature_request.md)
automatically. It asks for the feature description, the real problem behind it,
your proposed solution, alternatives you considered, which component it touches
(`core/`, `panel/`, `agent/`, the `vkai` CLI, the installer or the docs), and
whether it affects authentication, RBAC, panel access or the database schema.

## Security Issues

### Reporting Security Vulnerabilities

**DO NOT** create a public issue for security vulnerabilities.

Instead, please report security vulnerabilities by emailing:

**security@hitechcloud.vn**

**Response Timeline**:
- Initial Response: Within 24 hours
- Triage: Within 72 hours
- Fix Development: Within 7 days for critical vulnerabilities
- Public Disclosure: After fix is released

See our [Security Policy](SECURITY.md) for more details.

## Contributing

### How to Contribute

We welcome contributions! See our [Contributing Guide](docs/CONTRIBUTING.md) for details.

**Ways to Contribute**:
- Report bugs
- Suggest features
- Write documentation
- Submit code
- Review pull requests
- Help others

### Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## Feedback

### How to Provide Feedback

We value your feedback! You can provide feedback through:

- **GitHub Discussions**: https://github.com/hitechcloud-vietnam/vkai-panel/discussions
- **Email**: feedback@hitechcloud.vn
- **Survey**: https://forms.gle/xxx

### Feedback Categories

- **General Feedback**: General thoughts and opinions
- **Feature Feedback**: Feedback on specific features
- **Usability Feedback**: Feedback on user experience
- **Performance Feedback**: Feedback on performance
- **Documentation Feedback**: Feedback on documentation

## Community Guidelines

### Be Respectful

- Treat everyone with respect
- Be patient and understanding
- Avoid personal attacks
- Be constructive in feedback

### Be Helpful

- Help others when you can
- Share your knowledge
- Be patient with beginners
- Provide clear and helpful answers

### Be Constructive

- Provide constructive feedback
- Suggest improvements
- Be open to different perspectives
- Focus on solutions

## Contact Information

### General Support

- **Email**: support@hitechcloud.vn
- **Website**: https://hitechcloud.vn
- **GitHub**: https://github.com/hitechcloud-vietnam/vkai-panel

### Enterprise Support

- **Email**: enterprise@hitechcloud.vn
- **Phone**: +84 xxx xxx xxx
- **Website**: https://hitechcloud.vn/enterprise

### Security Issues

- **Email**: security@hitechcloud.vn
- **GitHub Security**: https://github.com/hitechcloud-vietnam/vkai-panel/security

### Community

- **Discord**: https://discord.gg/vkai-panel
- **Twitter**: https://twitter.com/hitechcloud
- **LinkedIn**: https://linkedin.com/company/hitechcloud

## Response Times

### Community Support

- **GitHub Discussions**: 24-48 hours
- **GitHub Issues**: 48-72 hours
- **Discord**: Real-time (community-based)

### Professional Support

- **Enterprise Support**: 4-8 hours
- **Consulting Services**: 24 hours

### Security Issues

- **Critical**: 24 hours
- **High**: 72 hours
- **Medium**: 1 week
- **Low**: 2 weeks

## Support Channels Summary

| Channel | Use Case | Response Time |
|---------|----------|---------------|
| Documentation | Self-service | Immediate |
| GitHub Discussions | General questions | 24-48 hours |
| GitHub Issues | Bug reports | 48-72 hours |
| Discord | Real-time support | Real-time |
| Stack Overflow | Technical questions | Community-based |
| Enterprise Support | Priority support | 4-8 hours |
| Security Email | Security issues | 24 hours |

## Frequently Asked Questions

### How do I get started?

See our [User Guide](docs/USER_GUIDE.md) for getting started instructions.

### How do I install VKAI Panel?

See our [Deployment Guide](docs/DEPLOYMENT.md) for installation instructions.

### How do I configure VKAI Panel?

See our [Configuration Guide](docs/CONFIGURATION.md) for configuration options.

### How do I report a bug?

See our [Bug Reports](#bug-reports) section for instructions.

### How do I request a feature?

See our [Feature Requests](#feature-requests) section for instructions.

### How do I contribute?

See our [Contributing Guide](docs/CONTRIBUTING.md) for instructions.

### How do I get enterprise support?

See our [Enterprise Support](#enterprise-support) section for instructions.

### How do I report a security vulnerability?

See our [Security Issues](#security-issues) section for instructions.

---

**Thank you for using VKAI Panel! We're here to help you succeed.**
