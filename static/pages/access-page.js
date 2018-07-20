import http from '../http.js';

const template = document.createElement('template')
template.innerHTML = `
    <div class="container">
        <h1>Messenger</h1>
        <form id="login-form" class="login-form">
            <input type="text" placeholder="Username" value="john" required>
            <button>Login</button>
        </form>
        <a href="/api/oauth/github" onclick="event.stopPropagation()">Access with GitHub</a>
    </div>
`

export default function accessPage() {
    const page = /** @type {DocumentFragment} */ (template.content.cloneNode(true))
    page.getElementById('login-form').onsubmit = onLoginSubmit
    return page
}

/**
 * @param {Event} ev
 */
async function onLoginSubmit(ev) {
    ev.preventDefault()

    const form = /** @type {HTMLFormElement} */ (ev.currentTarget)
    const input = form.querySelector('input')
    const submitButton = form.querySelector('button')

    input.disabled = true
    submitButton.disabled = true

    try {
        const payload = await login(input.value)
        input.value = ''
        localStorage.setItem('auth_user', JSON.stringify(payload.authUser))
        localStorage.setItem('token', payload.token)
        localStorage.setItem('expires_at', payload.expiresAt)
        location.reload()
    } catch (err) {
        console.error(err)
        alert(err.message)
        setTimeout(() => {
            input.focus()
        }, 0)
    } finally {
        input.disabled = false
        submitButton.disabled = false
    }
}

/**
 * @param {string} username
 */
function login(username) {
    return http.post('/api/login', { username })
}
