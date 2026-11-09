export interface IssueUser { login: string }
export interface GitHubIssue {
  number: number
  title: string
  body: string
  state: 'open' | 'closed'
  state_reason: string | null
  html_url: string
  user: IssueUser
  labels: { name: string; color: string }[]
  assignees: IssueUser[]
  comments: number
  locked: boolean
  created_at: string
  updated_at: string
}
export interface IssueComment { id: number; body: string; html_url: string; user: IssueUser; created_at: string; updated_at: string }
export interface IssuesPage { items: GitHubIssue[]; next_page: number; configured: boolean; repository: string }
export interface IssueCommentsPage { items: IssueComment[]; next_page: number }
