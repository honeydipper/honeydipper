# Function Map for Honeydipper v4

This document provides a structured overview of the functions available in the Honeydipper v4 branch, their purposes, and their interactions.

## Table of Contents
- [Functions](#functions)
  - [GitHub Operations](#github-operations)
  - [Issue Management](#issue-management)
  - [Pull Request Management](#pull-request-management)
  - [Repository Management](#repository-management)
  - [Code Management](#code-management)
  - [Search Operations](#search-operations)
  - [User and Team Management](#user-and-team-management)
  - [Copilot Integration](#copilot-integration)
  - [Secret Scanning](#secret-scanning)
  - [Internal Functions](#internal-functions)
- [Function Interactions](#function-interactions)

---

## Functions

### GitHub Operations

#### `mcp__github__get_me`
- **Purpose**: Retrieve details of the authenticated GitHub user.
- **Use Case**: Use this to fetch user information when building other tool calls or confirming user details.

#### `mcp__github__get_teams`
- **Purpose**: Retrieve details of the teams the user is a member of.
- **Use Case**: Useful for identifying team memberships and permissions.

#### `mcp__github__get_team_members`
- **Purpose**: Retrieve member usernames of a specific team in an organization.
- **Use Case**: Useful for identifying team members and their roles.

---

### Issue Management

#### `mcp__github__issue_read`
- **Purpose**: Retrieve information about a specific issue in a GitHub repository.
- **Methods**:
  - `get`: Get details of a specific issue.
  - `get_comments`: Get comments on an issue.
  - `get_sub_issues`: Get sub-issues of an issue.
  - `get_labels`: Get labels assigned to an issue.
- **Use Case**: Use this to fetch issue details, comments, sub-issues, or labels.

#### `mcp__github__issue_write`
- **Purpose**: Create a new issue or update an existing issue in a GitHub repository.
- **Methods**:
  - `create`: Create a new issue.
  - `update`: Update an existing issue.
- **Use Case**: Use this to create or update issues, including setting labels, assignees, and other metadata.

#### `mcp__github__add_issue_comment`
- **Purpose**: Add a comment to a specific issue in a GitHub repository.
- **Use Case**: Use this to add comments to issues or pull requests.

#### `mcp__github__list_issues`
- **Purpose**: List issues in a GitHub repository.
- **Use Case**: Use this to fetch a list of issues, filtered by labels, state, or other criteria.

#### `mcp__github__search_issues`
- **Purpose**: Search for issues in GitHub repositories using GitHub's issue search syntax.
- **Use Case**: Use this to search for issues across repositories.

#### `mcp__github__sub_issue_write`
- **Purpose**: Add or manage sub-issues of a parent issue.
- **Methods**:
  - `add`: Add a sub-issue to a parent issue.
  - `remove`: Remove a sub-issue from a parent issue.
  - `reprioritize`: Change the order of sub-issues within a parent issue.
- **Use Case**: Use this to manage hierarchical relationships between issues.

---

### Pull Request Management

#### `mcp__github__pull_request_read`
- **Purpose**: Retrieve information about a specific pull request in a GitHub repository.
- **Methods**:
  - `get`: Get details of a specific pull request.
  - `get_diff`: Get the diff of a pull request.
  - `get_status`: Get the combined commit status of a head commit in a pull request.
  - `get_files`: Get the list of files changed in a pull request.
  - `get_review_comments`: Get review threads on a pull request.
  - `get_reviews`: Get the reviews on a pull request.
  - `get_comments`: Get comments on a pull request.
  - `get_check_runs`: Get check runs for the head commit of a pull request.
- **Use Case**: Use this to fetch pull request details, diffs, statuses, files, reviews, or comments.

#### `mcp__github__pull_request_write`
- **Purpose**: Create or manage reviews of a pull request.
- **Methods**:
  - `create`: Create a new review of a pull request.
  - `submit_pending`: Submit an existing pending review.
  - `delete_pending`: Delete an existing pending review.
  - `resolve_thread`: Resolve a review thread.
  - `unresolve_thread`: Unresolve a previously resolved review thread.
- **Use Case**: Use this to create, submit, or delete pull request reviews, or manage review threads.

#### `mcp__github__create_pull_request`
- **Purpose**: Create a new pull request in a GitHub repository.
- **Use Case**: Use this to create a new pull request with specified base and head branches.

#### `mcp__github__update_pull_request`
- **Purpose**: Update an existing pull request in a GitHub repository.
- **Use Case**: Use this to update the title, body, base branch, or other metadata of a pull request.

#### `mcp__github__update_pull_request_branch`
- **Purpose**: Update the branch of a pull request with the latest changes from the base branch.
- **Use Case**: Use this to ensure a pull request branch is up-to-date with the base branch.

#### `mcp__github__merge_pull_request`
- **Purpose**: Merge a pull request in a GitHub repository.
- **Use Case**: Use this to merge a pull request into its base branch.

#### `mcp__github__list_pull_requests`
- **Purpose**: List pull requests in a GitHub repository.
- **Use Case**: Use this to fetch a list of pull requests, filtered by state, base branch, or other criteria.

#### `mcp__github__search_pull_requests`
- **Purpose**: Search for pull requests in GitHub repositories using GitHub's pull request search syntax.
- **Use Case**: Use this to search for pull requests across repositories.

#### `mcp__github__add_comment_to_pending_review`
- **Purpose**: Add a review comment to the requester's latest pending pull request review.
- **Use Case**: Use this to add comments to a pending review.

#### `mcp__github__add_reply_to_pull_request_comment`
- **Purpose**: Add a reply to an existing pull request comment.
- **Use Case**: Use this to reply to comments on a pull request.

---

### Repository Management

#### `mcp__github__list_repository_collaborators`
- **Purpose**: List collaborators of a GitHub repository.
- **Use Case**: Use this to fetch a list of collaborators, filtered by affiliation.

#### `mcp__github__list_branches`
- **Purpose**: List branches in a GitHub repository.
- **Use Case**: Use this to fetch a list of branches in a repository.

#### `mcp__github__create_branch`
- **Purpose**: Create a new branch in a GitHub repository.
- **Use Case**: Use this to create a new branch from an existing branch.

#### `mcp__github__fork_repository`
- **Purpose**: Fork a GitHub repository to your account or specified organization.
- **Use Case**: Use this to create a fork of a repository.

#### `mcp__github__create_repository`
- **Purpose**: Create a new GitHub repository in your account or specified organization.
- **Use Case**: Use this to create a new repository with specified settings.

#### `mcp__github__get_file_contents`
- **Purpose**: Get the contents of a file or directory from a GitHub repository.
- **Use Case**: Use this to fetch the contents of a file or directory.

#### `mcp__github__create_or_update_file`
- **Purpose**: Create or update a single file in a GitHub repository.
- **Use Case**: Use this to create or update a file in a repository.

#### `mcp__github__push_files`
- **Purpose**: Push multiple files to a GitHub repository in a single commit.
- **Use Case**: Use this to push multiple files to a repository.

#### `mcp__github__delete_file`
- **Purpose**: Delete a file from a GitHub repository.
- **Use Case**: Use this to delete a file from a repository.

---

### Code Management

#### `mcp__github__list_commits`
- **Purpose**: List commits in a GitHub repository.
- **Use Case**: Use this to fetch a list of commits, filtered by author, path, or date range.

#### `mcp__github__get_commit`
- **Purpose**: Get details for a commit from a GitHub repository.
- **Use Case**: Use this to fetch details of a specific commit, including diffs and stats.

#### `mcp__github__search_commits`
- **Purpose**: Search for commits across GitHub repositories using GitHub's commit search syntax.
- **Use Case**: Use this to search for commits across repositories.

#### `mcp__github__list_tags`
- **Purpose**: List git tags in a GitHub repository.
- **Use Case**: Use this to fetch a list of tags in a repository.

#### `mcp__github__get_tag`
- **Purpose**: Get details about a specific git tag in a GitHub repository.
- **Use Case**: Use this to fetch details of a specific tag.

#### `mcp__github__list_releases`
- **Purpose**: List releases in a GitHub repository.
- **Use Case**: Use this to fetch a list of releases in a repository.

#### `mcp__github__get_latest_release`
- **Purpose**: Get the latest release in a GitHub repository.
- **Use Case**: Use this to fetch the latest release in a repository.

#### `mcp__github__get_release_by_tag`
- **Purpose**: Get a specific release by its tag name in a GitHub repository.
- **Use Case**: Use this to fetch a specific release by its tag name.

---

### Search Operations

#### `mcp__github__search_code`
- **Purpose**: Search for code across GitHub repositories using GitHub's native search engine.
- **Use Case**: Use this to search for specific symbols, functions, classes, or code patterns.

#### `mcp__github__search_repositories`
- **Purpose**: Search for GitHub repositories by name, description, readme, topics, or other metadata.
- **Use Case**: Use this to discover projects or locate specific repositories.

#### `mcp__github__search_users`
- **Purpose**: Search for GitHub users by username, real name, or other profile information.
- **Use Case**: Use this to locate developers, contributors, or team members.

---

### User and Team Management

#### `mcp__github__get_teams`
- **Purpose**: Retrieve details of the teams the user is a member of.
- **Use Case**: Use this to identify team memberships and permissions.

#### `mcp__github__get_team_members`
- **Purpose**: Retrieve member usernames of a specific team in an organization.
- **Use Case**: Use this to identify team members and their roles.

---

### Copilot Integration

#### `mcp__github__request_copilot_review`
- **Purpose**: Request a GitHub Copilot code review for a pull request.
- **Use Case**: Use this to request automated feedback on pull requests before requesting human reviewers.

#### `mcp__github__assign_copilot_to_issue`
- **Purpose**: Assign Copilot to a specific issue in a GitHub repository.
- **Use Case**: Use this to delegate tasks to Copilot for automated resolution.

#### `mcp__github__create_pull_request_with_copilot`
- **Purpose**: Delegate a task to GitHub Copilot coding agent to perform in the background.
- **Use Case**: Use this to create a pull request with the implementation of a specific task.

#### `mcp__github__get_copilot_job_status`
- **Purpose**: Get the status of a GitHub Copilot coding agent job.
- **Use Case**: Use this to check the status of a previously submitted task.

---

### Secret Scanning

#### `mcp__github__run_secret_scanning`
- **Purpose**: Scan files, content, or recent changes for secrets such as API keys, passwords, tokens, and credentials.
- **Use Case**: Use this to scan specific files or content for secrets.

---

### Internal Functions

#### `mcp__internal__execute_bash_command`
- **Purpose**: Execute a bash command on the local machine.
- **Use Case**: Use this to run shell commands, scripts, or perform local operations.

#### `mcp__internal__read_file`
- **Purpose**: Read the contents of a file on the local machine.
- **Use Case**: Use this to fetch the contents of a local file.

#### `mcp__internal__write_file`
- **Purpose**: Write content to a file on the local machine.
- **Use Case**: Use this to create or update a local file.

#### `mcp__internal__append_to_file`
- **Purpose**: Append content to an existing file on the local machine.
- **Use Case**: Use this to add content to the end of a local file.

#### `mcp__internal__delete_file`
- **Purpose**: Delete a file from the local machine.
- **Use Case**: Use this to remove a local file.

#### `mcp__internal__list_files`
- **Purpose**: List files in a directory on the local machine.
- **Use Case**: Use this to fetch a list of files in a local directory.

#### `mcp__internal__create_directory`
- **Purpose**: Create a directory on the local machine.
- **Use Case**: Use this to create a new local directory.

#### `mcp__internal__delete_directory`
- **Purpose**: Delete a directory from the local machine.
- **Use Case**: Use this to remove a local directory.

#### `mcp__internal__execute_python_code`
- **Purpose**: Execute Python code on the local machine.
- **Use Case**: Use this to run Python scripts or perform Python-based operations.

#### `mcp__internal__install_python_package`
- **Purpose**: Install a Python package on the local machine.
- **Use Case**: Use this to install Python dependencies or libraries.

#### `mcp__internal__http_request`
- **Purpose**: Make an HTTP request to a specified URL.
- **Use Case**: Use this to interact with web APIs or fetch data from the internet.

---

## Function Interactions

### Creating and Managing Issues
1. **Create an Issue**: Use `mcp__github__issue_write` with the `create` method to create a new issue.
2. **Add Comments**: Use `mcp__github__add_issue_comment` to add comments to the issue.
3. **Update Issue**: Use `mcp__github__issue_write` with the `update` method to update the issue.
4. **List Issues**: Use `mcp__github__list_issues` to fetch a list of issues.

### Creating and Managing Pull Requests
1. **Create a Pull Request**: Use `mcp__github__create_pull_request` to create a new pull request.
2. **Add Comments**: Use `mcp__github__add_comment_to_pending_review` to add review comments.
3. **Update Pull Request**: Use `mcp__github__update_pull_request` to update the pull request.
4. **Merge Pull Request**: Use `mcp__github__merge_pull_request` to merge the pull request.

### Managing Repository Files
1. **Create or Update File**: Use `mcp__github__create_or_update_file` to create or update a file.
2. **Push Multiple Files**: Use `mcp__github__push_files` to push multiple files in a single commit.
3. **Delete File**: Use `mcp__github__delete_file` to delete a file.

### Searching and Retrieving Information
1. **Search Code**: Use `mcp__github__search_code` to search for specific code patterns.
2. **Search Issues**: Use `mcp__github__search_issues` to search for issues.
3. **Search Pull Requests**: Use `mcp__github__search_pull_requests` to search for pull requests.

### Copilot Integration
1. **Request Copilot Review**: Use `mcp__github__request_copilot_review` to request a Copilot review for a pull request.
2. **Assign Copilot to Issue**: Use `mcp__github__assign_copilot_to_issue` to assign Copilot to an issue.
3. **Check Job Status**: Use `mcp__github__get_copilot_job_status` to check the status of a Copilot job.

### Internal Functions
1. **Execute Bash Command**: Use `mcp__internal__execute_bash_command` to run shell commands.
2. **Read File**: Use `mcp__internal__read_file` to fetch the contents of a local file.
3. **Write File**: Use `mcp__internal__write_file` to create or update a local file.
4. **Append to File**: Use `mcp__internal__append_to_file` to add content to a local file.
5. **Delete File**: Use `mcp__internal__delete_file` to remove a local file.
6. **List Files**: Use `mcp__internal__list_files` to fetch a list of files in a directory.
7. **Create Directory**: Use `mcp__internal__create_directory` to create a local directory.
8. **Delete Directory**: Use `mcp__internal__delete_directory` to remove a local directory.
9. **Execute Python Code**: Use `mcp__internal__execute_python_code` to run Python scripts.
10. **Install Python Package**: Use `mcp__internal__install_python_package` to install Python dependencies.
11. **HTTP Request**: Use `mcp__internal__http_request` to interact with web APIs.

---

This document provides a comprehensive overview of the functions available in the Honeydipper v4 branch and their interactions. Use this as a reference for understanding how to interact with the repository and its features.