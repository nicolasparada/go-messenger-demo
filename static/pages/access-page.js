const template = document.createElement('template')
template.innerHTML = `
    <div class="container">
        <h1>Messenger</h1>
        <a href="/api/oauth/github" onclick="event.stopPropagation()">Access with GitHub</a>
    </div>
`

export default function accessPage() {
    return template.content
}
