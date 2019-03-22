import http from '../http.js';

const template = document.createElement('template')
template.innerHTML = `
    <div class="container">
        <h1>Messenger</h1>
        <form id="login-form">
            <input type="text" placeholder="Username" value="john" required>
            <button>Login</button>
        </form>
    </div>
`

export default function accessPage() {
    const page = /** @type {DocumentFragment} */ (template.content.cloneNode(true))
    const loginForm = /** @type {HTMLFormElement} */ (page.getElementById('login-form'))
    const loginFormInput = loginForm.querySelector('input')
    const loginFormButton = loginForm.querySelector('button')

    /**
     * @param {Event} ev
     */
    const onLoginFormSubmit = async ev => {
        ev.preventDefault()
        const username = loginFormInput.value
        loginFormInput.disabled = true
        loginFormButton.disabled = true
        try {
            const payload = await login(username)
            localStorage.setItem('auth_user', JSON.stringify(payload.authUser))
            localStorage.setItem('token', payload.token)
            localStorage.setItem('expires_at', payload.expiresAt)
            loginForm.reset()
            location.reload()
        } catch (err) {
            console.error(err)
            alert(err.message)
            setTimeout(() => {
                loginFormInput.focus()
            })
        } finally {
            loginFormInput.disabled = false
            loginFormButton.disabled = false
        }
    }

    loginForm.addEventListener('submit', onLoginFormSubmit)

    return page
}

/**
 * @param {string} username
 * @returns {Promise<{authUser:{id:string,username:string,avatarURL:string},token:string,expiresAt:string}>}
 */
function login(username) {
    return http.post('/api/login', { username })
}
