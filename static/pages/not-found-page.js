const template = document.createElement('template')
template.innerHTML = `
    <div class="container">
        <h1>404 Not Found</h1>
    </div>
`

export default function notFound() {
    return template.content
}
