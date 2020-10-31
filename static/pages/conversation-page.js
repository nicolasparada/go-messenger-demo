import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.8.0/router.js';
import http from '../http.js';
import { ago, avatar, escapeHTML, flashTitle, linkify, loadEventSourcePolyfill } from '../shared.js';

// ConversationPage is a custom element
// so it takes advantage of disconnectedCallback
// to unsubscribe from the messages.
class ConversationPage extends HTMLElement {
    /**
     * @param {string} conversationId
     */
    constructor(conversationId) {
        super()

        this.conversationId = conversationId

        this.onBackLinkClick = this.onBackLinkClick.bind(this)
        this.onLoadMoreClick = this.onLoadMoreClick.bind(this)
        this.onMessageSubmit = this.onMessageSubmit.bind(this)
        this.onMessageArrive = this.onMessageArrive.bind(this)
    }

    /**
     * @param {MouseEvent} ev
     */
    onBackLinkClick(ev) {
        ev.preventDefault()
        history.back()
    }

    async onLoadMoreClick() {
        const before = this.loadMoreButton.dataset['before']

        this.loadMoreButton.disabled = true
        const messages = await getMessages(this.conversationId, before).catch(err => {
            console.error(err)
            return []
        })
        this.loadMoreButton.disabled = false

        const firstLI = this.loadMoreButton.parentElement
        for (const m of messages) {
            firstLI.insertAdjacentElement('afterend', renderMessage(m))
        }

        if (messages.length !== 25) {
            this.loadMoreButton.remove()
            return
        }

        this.loadMoreButton.dataset['before'] = messages[24].i
    }

    /**
     * @param {Event} ev
     */
    async onMessageSubmit(ev) {
        ev.preventDefault()

        this.messageInput.disabled = true
        this.messageSubmitButton.disabled = true

        try {
            const m = await createMessage(this.messageInput.value, this.conversationId)
            this.messageInput.value = ''
            const messagesOList = document.getElementById('messages')
            if (messagesOList !== null) {
                messagesOList.appendChild(renderMessage(m))
                setTimeout(() => {
                    messagesOList.scrollTop = messagesOList.scrollHeight
                })
            }
        } catch (err) {
            if (err.statusCode === 422) {
                this.messageInput.setCustomValidity(err.body.errors.content)
            } else {
                alert(err.message)
            }
        } finally {
            this.messageInput.disabled = false
            this.messageSubmitButton.disabled = false
            setTimeout(() => {
                this.messageInput.focus()
            })
        }
    }

    onMessageArrive(message) {
        flashTitle(message.content.substr(0, 20) + '...')

        if (message.conversationId !== this.conversationId) {
            return
        }

        this.messagesOList.appendChild(renderMessage(message))
        const isAtTheBottom = this.messagesOList.scrollTop + this.messagesOList.clientHeight === this.messagesOList.scrollHeight
        if (isAtTheBottom) {
            setTimeout(() => {
                this.messagesOList.scrollTop = this.messagesOList.scrollHeight
            })
        }
        readMessages(message.conversationId)
    }

    async connectedCallback() {
        let otherParticipant, messages
        try {
            [otherParticipant, messages] = await Promise.all([
                getOtherParticipantFromConversation(this.conversationId),
                getMessages(this.conversationId),
            ])
            this.unsubscribeFromMessages = await subscribeToMessages(this.onMessageArrive)
        } catch (err) {
            alert(err.message)
            navigate('/', true)
            return
        }

        const messagesLength = messages.length
        const showLoadMoreButton = messagesLength === 25
        const lastMessage = messages[messagesLength - 1]

        const template = document.createElement('template')
        template.innerHTML = `
            <div class="chat container">
                <div class="chat-heading">
                    <a href="/" id="back-link" class="back-link">← Back</a>
                    <div class="avatar-wrapper">
                        ${avatar(otherParticipant)}
                        <span>${otherParticipant.username}</span>
                    </div>
                </div>
                <ol id="messages" class="messages">${showLoadMoreButton
                ? `<li class="load-more">
                    <button id="load-more-button" data-before="${lastMessage.id}">Load more</button>
                </li>`
                : ''}</ol>
                <form id="message-form" class="message-form">
                    <input type="text" placeholder="Type something" maxlength="480" required>
                    <button>Send</button>
                </form>
            </div>
        `
        this.appendChild(template.content)
        this.backLink = /** @type {HTMLAnchorElement} */ (this.querySelector('#back-link'))
        this.messagesOList = /** @type {HTMLOListElement} */ (this.querySelector('#messages'))
        this.loadMoreButton = /** @type {HTMLButtonElement=} */ (this.querySelector('#load-more-button'))
        this.messageForm = /** @type {HTMLFormElement} */ (this.querySelector('#message-form'))
        this.messageInput = this.messageForm.querySelector('input')
        this.messageSubmitButton = this.messageForm.querySelector('button')

        this.backLink.onclick = this.onBackLinkClick
        this.messageForm.onsubmit = this.onMessageSubmit

        if (showLoadMoreButton) {
            this.loadMoreButton.onclick = this.onLoadMoreClick
        }

        for (const m of messages.reverse()) {
            this.messagesOList.appendChild(renderMessage(m))
        }

        setTimeout(() => {
            this.messagesOList.scrollTop = this.messagesOList.scrollHeight
        })
    }

    disconnectedCallback() {
        if (typeof this.unsubscribeFromMessages === 'function') {
            this.unsubscribeFromMessages()
        }
    }
}

customElements.define('conversation-page', ConversationPage)

export default params => new ConversationPage(params[0])

/**
 * @param {string} conversationId
 */
function getOtherParticipantFromConversation(conversationId) {
    return http.get(`/api/conversations/${conversationId}/other_participant`)
}

/**
 * @param {string} conversationId
 * @param {string=} before
 */
function getMessages(conversationId, before) {
    let url = `/api/conversations/${conversationId}/messages`
    if (typeof before === 'string' && before !== '') {
        url += '?before=' + before
    }
    return http.get(url)
}


function renderMessage(message) {
    const li = document.createElement('li')
    li.className = 'message'
    if (message.mine) {
        li.classList.add('owned')
    }
    li.innerHTML = `
        <div class="buble">
            <p>${linkify(escapeHTML(message.content))}</p>
        </div>
        <time>${ago(message.createdAt)}</time>
    `
    return li
}

/**
 * @param {string} content
 * @param {string} conversationId
 */
function createMessage(content, conversationId) {
    return http.post(`/api/conversations/${conversationId}/messages`, { content })
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

/**
 * @param {string} conversationId
 */
function readMessages(conversationId) {
    return http.post(`/api/conversations/${conversationId}/read_messages`)
}
