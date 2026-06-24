<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>{{.Title}}</title>
<script src="https://cdn.tailwindcss.com"></script>
<style>
  body {
    font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans SC', sans-serif;
    font-size: 14px;
    line-height: 1.8;
    color: #1f2937;
    -webkit-print-color-adjust: exact;
    print-color-adjust: exact;
  }
  .article-title {
    font-size: 28px;
    font-weight: 700;
    color: #111827;
    margin-bottom: 1rem;
    padding-bottom: 0.75rem;
    border-bottom: 2px solid #e5e7eb;
  }
  .article-summary {
    font-size: 14px;
    color: #6b7280;
    margin-bottom: 2rem;
  }
  .article-content {
    line-height: 1.8;
    color: #374151;
    word-break: break-all;
    overflow-wrap: break-word;
  }
  .article-content h1 { font-size: 1.25rem; font-weight: 600; margin-top: 1.5em; margin-bottom: 0.75em; color: #1f2937; }
  .article-content h2 { font-size: 1.125rem; font-weight: 600; margin-top: 1.5em; margin-bottom: 0.75em; color: #1f2937; }
  .article-content h3 { font-size: 1rem; font-weight: 600; margin-top: 1.25em; margin-bottom: 0.5em; color: #1f2937; }
  .article-content h4 { font-size: 0.9375rem; font-weight: 600; margin-top: 1em; margin-bottom: 0.5em; color: #1f2937; }
  .article-content p { margin-bottom: 1em; }
  .article-content a { color: #8b5cf6; text-decoration: none; }
  .article-content a:hover { text-decoration: underline; }
  .article-content ul, .article-content ol { margin-bottom: 1em; padding-left: 2em; }
  .article-content li { margin-bottom: 0.5em; }
  .article-content blockquote {
    margin: 1em 0; padding: 0.75em 1em;
    background: linear-gradient(to right, #f3e8ff, #fce7f3);
    border-left: 4px solid #8b5cf6;
    border-radius: 0 0.5rem 0.5rem 0;
    color: #6b21a8;
  }
  .article-content code {
    background: #f3f4f6; padding: 0.125em 0.375em;
    border-radius: 0.25rem; font-family: 'Monaco', 'Consolas', monospace;
    font-size: 0.875em; color: #7c3aed;
  }
  .article-content pre {
    margin: 1em 0; padding: 1em; background: #1f2937;
    border-radius: 0.5rem; overflow-x: auto;
  }
  .article-content pre code { background: transparent; padding: 0; color: #e5e7eb; }
  .article-content img { max-width: 100%; border-radius: 0.5rem; margin: 1em 0; }
  .article-content table { width: 100%; border-collapse: collapse; margin: 1em 0; }
  .article-content th, .article-content td { border: 1px solid #e5e7eb; padding: 0.5em 1em; text-align: left; }
  .article-content th { background: linear-gradient(to right, #f3e8ff, #fce7f3); font-weight: 600; color: #6b21a8; }
  .article-content hr { margin: 1.5em 0; border: none; border-top: 1px solid #e5e7eb; }
  .article-content strong { font-weight: 600; color: #1f2937; }
  .article-content em { font-style: italic; }
  .article-content del { text-decoration: line-through; color: #9ca3af; }
  .article-content pre, .article-content table { page-break-inside: avoid; }
  .article-content img { page-break-inside: avoid; }

  /* 评论区 */
  .comments-section {
    margin-top: 3rem;
    padding-top: 1.5rem;
    border-top: 2px solid #e5e7eb;
  }
  .comments-title {
    font-size: 18px;
    font-weight: 600;
    color: #374151;
    margin-bottom: 1.5rem;
    display: flex;
    align-items: center;
    gap: 0.5rem;
  }
  .comment-card {
    background: #f9fafb;
    border-radius: 0.75rem;
    padding: 1rem;
    margin-bottom: 1rem;
    page-break-inside: avoid;
  }
  .comment-header {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    margin-bottom: 0.5rem;
  }
  .comment-avatar {
    width: 32px;
    height: 32px;
    border-radius: 50%;
    object-fit: cover;
  }
  .comment-user {
    font-weight: 500;
    color: #374151;
    font-size: 14px;
  }
  .comment-meta {
    font-size: 12px;
    color: #9ca3af;
    margin-left: 0.5rem;
  }
  .comment-body {
    font-size: 14px;
    color: #4b5563;
    margin-bottom: 0.5rem;
    line-height: 1.6;
  }
  .comment-footer {
    font-size: 12px;
    color: #9ca3af;
  }
  .discussion-block {
    margin-left: 1.5rem;
    margin-top: 0.75rem;
    padding-left: 0.75rem;
    border-left: 2px solid #e9d5ff;
  }
  .discussion-item {
    background: #fff;
    border-radius: 0.5rem;
    padding: 0.75rem;
    margin-bottom: 0.5rem;
    border: 1px solid #f3f4f6;
    page-break-inside: avoid;
  }
  .discussion-user {
    font-size: 13px;
    font-weight: 500;
    color: #374151;
  }
  .discussion-content {
    font-size: 13px;
    color: #4b5563;
    margin-top: 0.25rem;
    line-height: 1.6;
  }
  .child-discussions {
    margin-left: 1.5rem;
    margin-top: 0.5rem;
    padding-left: 0.75rem;
    border-left: 2px solid #fce7f3;
  }
  .child-discussion {
    background: #f9fafb;
    border-radius: 0.375rem;
    padding: 0.5rem 0.75rem;
    margin-bottom: 0.375rem;
    font-size: 12px;
  }
  .child-discussion .child-user {
    font-weight: 500;
    color: #374151;
  }
  .child-discussion .child-content {
    color: #6b7280;
    margin-top: 0.125rem;
  }
</style>
</head>
<body>
  <div class="article-title">{{.Title}}</div>
  {{if .Summary}}
  <div class="article-summary">{{.Summary}}</div>
  {{end}}
  <div class="article-content">{{.Content}}</div>

  {{if .Comments}}
  <div class="comments-section">
    <div class="comments-title">
      <svg xmlns="http://www.w3.org/2000/svg" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="#8b5cf6" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 15a2 2 0 0 1-2 2H7l-4 4V5a2 2 0 0 1 2-2h14a2 2 0 0 1 2 2z"/></svg>
      网友见解（{{len .Comments}} 条）
    </div>
    {{range .Comments}}
    <div class="comment-card">
      <div class="comment-header">
        {{if .UserHeader}}
        <img class="comment-avatar" src="{{.UserHeader}}" alt="{{.UserName}}" onerror="this.style.display='none'">
        {{end}}
        <span class="comment-user">{{.UserName}}</span>
        <span class="comment-meta">👍 {{.LikeCount}} · 💬 {{.DiscussionCount}} · {{.Time}}</span>
      </div>
      <div class="comment-body">{{.Content}}</div>
      {{if .Discussions}}
      <div class="discussion-block">
        {{range .Discussions}}
        <div class="discussion-item">
          <div class="comment-header">
            {{if .Avatar}}
            <img class="comment-avatar" src="{{.Avatar}}" alt="{{.Nickname}}" onerror="this.style.display='none'">
            {{end}}
            <span class="comment-user">{{.Nickname}}</span>
            {{if .ReplyNickname}}
            <span class="comment-meta">回复 {{.ReplyNickname}}</span>
            {{end}}
            <span class="comment-meta">{{.Time}}</span>
            {{if .LikesNumber}}
            <span class="comment-meta">👍 {{.LikesNumber}}</span>
            {{end}}
          </div>
          <div class="discussion-content">{{.Content}}</div>
          {{if .ChildDiscussions}}
          <div class="child-discussions">
            {{range .ChildDiscussions}}
            <div class="child-discussion">
              <span class="child-user">{{.AuthorNickname}}</span>
              {{if .ReplyNickname}}
              <span class="comment-meta">回复 {{.ReplyNickname}}</span>
              {{end}}
              <div class="child-content">{{.Content}}</div>
            </div>
            {{end}}
          </div>
          {{end}}
        </div>
        {{end}}
      </div>
      {{end}}
    </div>
    {{end}}
  </div>
  {{end}}
</body>
</html>
