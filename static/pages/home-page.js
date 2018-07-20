import { navigate } from 'https://unpkg.com/@nicolasparada/router@0.6.0/router.js';
import { getAuthUser } from '../auth.js';
import http from '../http.js';
import { ago, avatar, escapeHTML, loadEventSourcePolyfill } from '../shared.js';

export default async function homePage() {
    const conversations = await getConversations().catch(err => {
        console.error(err)
        return []
    })

    const conversationsLength = conversations.length
    const showLoadMoreButton = conversationsLength === 25
    const lastConversation = conversations[conversationsLength - 1]
    const authUser = getAuthUser()
    const template = document.createElement('template')
    template.innerHTML = /*html*/`
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
            ? /*html*/`<button id="load-more-button" data-before="${lastConversation.id}">Load more</button>`
            : ''}
        </div>
    `
    const page = template.content
    page.getElementById('logout-button').onclick = onLogoutClick
    page.getElementById('conversation-form').onsubmit = onConversationSubmit
    page.getElementById('username-input').oninput = onUsernameInput
    const conversationsOList = page.getElementById('conversations')
    for (const c of conversations) {
        conversationsOList.appendChild(renderConversation(c))
    }
    if (showLoadMoreButton) {
        page.getElementById('load-more-button').onclick = onLoadMoreClick
    }
    page.addEventListener('disconnect', await subscribeToMessages(onMessageArrive))
    return page
}

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

function onLogoutClick() {
    localStorage.clear()
    location.reload()
}

/**
 * @param {Event} ev
 */
async function onConversationSubmit(ev) {
    ev.preventDefault()

    const form = /** @type {HTMLFormElement} */ (ev.currentTarget)
    const input = form.querySelector('input')

    input.disabled = true

    try {
        const conversation = await createConversation(input.value)
        input.value = ''
        navigate('/conversations/' + conversation.id)
    } catch (err) {
        if (err.statusCode === 422) {
            input.setCustomValidity(err.body.errors.username)
        } else {
            alert(err.message)
        }
        setTimeout(() => {
            input.focus()
        }, 0)
    } finally {
        input.disabled = false
    }
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

let searchingUsernames = false

/**
 * @param {Event} ev
 */
async function onUsernameInput(ev) {
    if (searchingUsernames) {
        return
    }

    const input = /** @type {HTMLInputElement} */ (ev.currentTarget)
    const search = input.value.trim()

    if (search === '') {
        return
    }

    searchingUsernames = true
    const usernames = await searchUsernames(search).catch(err => {
        console.error(err)
        return []
    })
    searchingUsernames = false

    const usernamesDataList = /** @type {HTMLDataListElement} */ (document.getElementById('usernames-datalist'))
    if (usernamesDataList === null) {
        return
    }

    usernamesDataList.innerHTML = usernames
        .map(username => `<option value="${username}">${username}</option>`)
        .join('')
}

function searchUsernames(search) {
    return http.get('/api/usernames?search=' + search)
}

/**
 * @param {MouseEvent} ev
 */
async function onLoadMoreClick(ev) {
    const button = /** @type {HTMLButtonElement} */ (ev.currentTarget)
    const before = button.dataset['before']

    button.disabled = true

    const conversations = await getConversations(before).catch(err => {
        console.error(err)
        return []
    })

    button.disabled = false

    const conversationsOList = document.getElementById('conversations')
    if (conversationsOList !== null) {
        for (const c of conversations) {
            conversationsOList.appendChild(renderConversation(c))
        }
    }

    const conversationsLength = conversations.length
    if (conversationsLength !== 25) {
        button.remove()
        return
    }

    button.dataset['before'] = conversations[conversationsLength - 1].id
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

async function onMessageArrive(message) {
    const conversationItem = document.querySelector(`.conversation[data-id="${message.conversationId}"]`)
    if (conversationItem !== null) {
        conversationItem.classList.add('has-unread-messages')
        conversationItem.querySelector('.message-preview p').textContent = message.content
        conversationItem.querySelector('.message-preview time').textContent = ago(message.createdAt)
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

    const conversationsOList = document.getElementById('conversations')
    if (conversationsOList === null) {
        return
    }

    conversationsOList.insertAdjacentElement('afterbegin', renderConversation(conversation))
}

function getConversation(id) {
    return http.get('/api/conversations/' + id)
}
