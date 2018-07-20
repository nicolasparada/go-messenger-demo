import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.6.0/router.js';
import http from '../http.js';
import { ago, avatar, escapeHTML, linkify, loadEventSourcePolyfill } from '../shared.js';

export default async function conversationPage(conversationId) {
    let conversation, messages
    try {
        [conversation, messages] = await Promise.all([
            getConversation(conversationId),
            getMessages(conversationId),
        ])
    } catch (err) {
        alert(err.message)
        navigate('/', true)
        return
    }

    const messagesLength = messages.length
    const showLoadMoreButton = messagesLength === 25
    const lastMessage = messages[messagesLength - 1]
    const template = document.createElement('template')
    template.innerHTML = /*html*/`
        <div class="chat container">
            <div class="chat-heading">
                <a href="/" id="back-link" class="back-link">← Back</a>
                <div class="avatar-wrapper">
                    ${avatar(conversation.otherParticipant)}
                    <span>${conversation.otherParticipant.username}</span>
                </div>
            </div>
            <ol id="messages" class="messages">${showLoadMoreButton
            ? /*html*/`<li class="load-more">
                <button id="load-more-button" data-before="${lastMessage.id}">Load more</button>
            </li>`
            : ''}</ol>
            <form id="message-form" class="message-form">
                <input type="text" placeholder="Type something" maxlength="480" required>
                <button>Send</button>
            </form>
        </div>
    `
    const page = template.content
    page.getElementById('back-link').onclick = onBackLinkClick
    const loadMoreButton = page.getElementById('load-more-button')
    if (loadMoreButton !== null) {
        loadMoreButton.onclick = loadMoreClicker(conversationId)
    }
    const messagesOList = page.getElementById('messages')
    for (const m of messages) {
        if (loadMoreButton !== null) {
            loadMoreButton.parentElement.insertAdjacentElement('beforebegin', renderMessage(m))
        } else {
            messagesOList.appendChild(renderMessage(m))
        }
    }
    page.getElementById('message-form').onsubmit = messageSubmitter(conversationId)
    page.addEventListener('disconnect', await subscribeToMessages(messageArriver(conversationId)))
    return page
}

/**
 * @param {string} id
 */
function getConversation(id) {
    return http.get('/api/conversations/' + id)
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

/**
 * @param {MouseEvent} ev
 */
function onBackLinkClick(ev) {
    ev.preventDefault()
    history.back()
}

function renderMessage(message) {
    const li = document.createElement('li')
    li.className = 'message'
    if (message.mine) {
        li.classList.add('owned')
    }
    li.innerHTML = /*html*/`
        <div class="buble">
            <p>${linkify(escapeHTML(message.content))}</p>
        </div>
        <time>${ago(message.createdAt)}</time>
    `
    return li
}

/**
 * @param {string} conversationId
 * @returns {function(MouseEvent)}
 */
function loadMoreClicker(conversationId) {
    return async ev => {
        const button = /** @type {HTMLButtonElement} */ (ev.currentTarget)
        const before = button.dataset['before']

        button.disabled = true
        const messages = await getMessages(conversationId, before).catch(err => {
            console.error(err)
            return []
        })
        button.disabled = false

        const messagesOList = document.getElementById('messages')
        if (messagesOList !== null) {
            const lastMessageLI = messagesOList.querySelector('.message:nth-last-child(2)')
            for (const m of messages) {
                button.parentElement.insertAdjacentElement('beforebegin', renderMessage(m))
            }
            setTimeout(() => {
                if (lastMessageLI !== null) {
                    lastMessageLI.scrollIntoView()
                }
            }, 0)
        }


        const messagesLength = messages.length
        if (messagesLength !== 25) {
            button.remove()
        }

        button.dataset['before'] = messages[messagesLength - 1].i
    }
}

/**
 * @param {string} conversationId
 * @returns {function(Event)}
 */
function messageSubmitter(conversationId) {
    return async ev => {
        ev.preventDefault()
        const form = /** @type {HTMLFormElement} */ (ev.currentTarget)
        const input = form.querySelector('input')
        const submitButton = form.querySelector('button')

        input.disabled = true
        submitButton.disabled = true

        try {
            const m = await createMessage(input.value, conversationId)
            input.value = ''
            const messagesOList = document.getElementById('messages')
            if (messagesOList !== null) {
                messagesOList.insertAdjacentElement('afterbegin', renderMessage(m))
            }
        } catch (err) {
            if (err.statusCode === 422) {
                input.setCustomValidity(err.body.errors.content)
            } else {
                alert(err.message)
            }
        } finally {
            input.disabled = false
            submitButton.disabled = false
            setTimeout(() => {
                input.focus()
            }, 0)
        }
    }
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
function messageArriver(conversationId) {
    return message => {
        if (message.conversationId !== conversationId) {
            return
        }
        const messagesOList = document.getElementById('messages')
        if (messagesOList === null) {
            return
        }
        messagesOList.insertAdjacentElement('afterbegin', renderMessage(message))
        readMessages(message.conversationId)
    }
}

/**
 * @param {string} conversationId
 */
function readMessages(conversationId) {
    return http.post(`/api/conversations/${conversationId}/read_messages`)
}
