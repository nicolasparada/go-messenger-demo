import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.8.0/router.js';
import { getAuthUser } from '../auth.js';
import http from '../http.js';
import { ago, avatar, escapeHTML, loadEventSourcePolyfill } from '../shared.js';

// HomePage is a custom element
// so it takes advantage of disconnectedCallback
// to unsubscribe from the messages.
class HomePage extends HTMLElement {
    constructor() {
        super()

        this.searchingUsernames = false

        this.onLogoutClick = this.onLogoutClick.bind(this)
        this.onConversationSubmit = this.onConversationSubmit.bind(this)
        this.onUsernameInput = this.onUsernameInput.bind(this)
        this.onLoadMoreClick = this.onLoadMoreClick.bind(this)
        this.onMessageArrive = this.onMessageArrive.bind(this)
    }

    onLogoutClick() {
        localStorage.clear()
        location.reload()
    }

    /**
     * @param {Event} ev
     */
    async onConversationSubmit(ev) {
        ev.preventDefault()

        this.usernameInput.disabled = true

        try {
            const conversation = await createConversation(this.usernameInput.value)
            this.usernameInput.value = ''
            navigate('/conversations/' + conversation.id)
        } catch (err) {
            if (err.statusCode === 422) {
                this.usernameInput.setCustomValidity(err.body.errors.username)
            } else {
                alert(err.message)
            }
            setTimeout(() => {
                this.usernameInput.focus()
            })
        } finally {
            this.usernameInput.disabled = false
        }
    }

    /**
     * @param {Event} ev
     */
    async onUsernameInput(ev) {
        if (this.searchingUsernames) {
            return
        }

        const search = this.usernameInput.value.trim()

        if (search === '') {
            return
        }

        this.searchingUsernames = true
        const usernames = await searchUsernames(search).catch(err => {
            console.error(err)
            return []
        })
        this.searchingUsernames = false

        this.usernamesDataList.innerHTML = usernames
            .map(username => `<option value="${username}">${username}</option>`)
            .join('')
    }

    /**
     * @param {MouseEvent} ev
     */
    async onLoadMoreClick(ev) {
        const before = this.loadMoreButton.dataset['before']

        this.loadMoreButton.disabled = true

        const conversations = await getConversations(before).catch(err => {
            console.error(err)
            return []
        })

        this.loadMoreButton.disabled = false

        const conversationsOList = document.getElementById('conversations')
        if (conversationsOList !== null) {
            for (const c of conversations.reverse()) {
                conversationsOList.appendChild(renderConversation(c))
            }
        }

        const conversationsLength = conversations.length
        if (conversationsLength !== 25) {
            this.loadMoreButton.remove()
            return
        }

        this.loadMoreButton.dataset['before'] = conversations[conversationsLength - 1].id
    }

    async onMessageArrive(message) {
        const conversationLI = this.querySelector(`.conversation[data-id="${message.conversationId}"]`)
        if (conversationLI !== null) {
            conversationLI.classList.add('has-unread-messages')
            conversationLI.querySelector('.message-preview p').textContent = message.content
            conversationLI.querySelector('.message-preview time').textContent = ago(message.createdAt)
            return
        }

        let conversation
        try {
            conversation = await getConversation(message.conversationId)
            conversation.hasUnreadMessages = true
            conversation.lastMessage = message
        } catch (err) {
            console.error(err)
            return
        }

        this.conversationsOList.insertAdjacentElement('afterbegin', renderConversation(conversation))
    }

    async connectedCallback() {
        const conversations = await getConversations().catch(() => [])
        this.unsubscribeFromMessages = await subscribeToMessages(this.onMessageArrive)

        const conversationsLength = conversations.length
        const showLoadMoreButton = conversationsLength === 25
        const lastConversation = conversations[conversationsLength - 1]
        const authUser = getAuthUser()

        const template = document.createElement('template')
        template.innerHTML = `
            <div class="container">
                <section class="profile">
                    <div class="avatar-wrapper">
                        ${avatar(authUser)}
                        <span>${authUser.username}</span>
                        <button id="logout-button" class="logout-button">Logout</button>
                    </div>
                </section>
                <h2>Conversations</h2>
                <form id="conversation-form">
                    <input id="username-input" type="search" placeholder="Start conversation with..." list="usernames-datalist" required>
                    <datalist id="usernames-datalist"></datalist>
                </form>
                <ol id="conversations" class="conversations"></ol>
                ${showLoadMoreButton
                ? `<button id="load-more-button" data-before="${lastConversation.id}">Load more</button>`
                : ''}
            </div>
        `
        this.appendChild(template.content)
        this.logoutButton = /** @type {HTMLButtonElement} */ (this.querySelector('#logout-button'))
        this.conversationForm = /** @type {HTMLFormElement} */ (this.querySelector('#conversation-form'))
        this.usernameInput = /** @type {HTMLInputElement} */ (this.querySelector('#username-input'))
        this.usernamesDataList = /** @type {HTMLDataListElement} */ (this.querySelector('#usernames-datalist'))
        this.conversationsOList = /** @type {HTMLOListElement} */ (this.querySelector('#conversations'))
        this.loadMoreButton = /** @type {HTMLButtonElement=} */ (this.querySelector('#load-more-button'))

        this.logoutButton.onclick = this.onLogoutClick
        this.conversationForm.onsubmit = this.onConversationSubmit
        this.usernameInput.oninput = this.onUsernameInput

        for (const c of conversations) {
            this.conversationsOList.appendChild(renderConversation(c))
        }

        if (showLoadMoreButton) {
            this.loadMoreButton.onclick = this.onLoadMoreClick
        }
    }

    disconnectedCallback() {
        if (typeof this.unsubscribeFromMessages === 'function') {
            this.unsubscribeFromMessages()
        }
    }
}

customElements.define('home-page', HomePage)

export default () => new HomePage()

/**
 * @param {string=} before
 */
function getConversations(before) {
    let url = '/api/conversations'
    if (before) {
        url += '?before=' + before
    }
    return http.get(url)
}

/**
 * @param {string} username
 */
function createConversation(username) {
    return http.post('/api/conversations', { username })
}

function renderConversation(conversation) {
    const li = document.createElement('li')
    li.className = 'conversation'
    li.dataset['id'] = conversation.id
    if (conversation.hasUnreadMessages) {
        li.classList.add('has-unread-messages')
    }
    li.innerHTML = `
        <a href="/conversations/${conversation.id}">
            <div class="avatar-wrapper">
                ${avatar(conversation.otherParticipant)}
                <span>${conversation.otherParticipant.username}</span>
            </div>
            <div class="message-preview">
                <p>${conversation.lastMessage.mine ? 'You: ' : ''}${escapeHTML(conversation.lastMessage.content)}</p>
                <time>${ago(conversation.lastMessage.createdAt)}</time>
            </div>
        </a>
    `
    return li
}

function searchUsernames(search) {
    return http.get('/api/usernames?search=' + search)
}

/**
 * @param {function} cb
 */
async function subscribeToMessages(cb) {
    if (!('EventSource' in window)) {
        await loadEventSourcePolyfill()
    }
    return http.subscribe('/api/messages', cb)
}

function getConversation(id) {
    return http.get('/api/conversations/' + id)
}
