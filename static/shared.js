const urlRegExp = /\b((?:https?:\/\/|www\d{0,3}[.]|[a-z0-9.\-]+[.][a-z]{2,4}\/)(?:[^\s()<>]+|\(([^\s()<>]+|(\([^\s()<>]+\)))*\))+(?:\(([^\s()<>]+|(\([^\s()<>]+\)))*\)|[^\s`!()\[\]{};:'".,<>?«»“”‘’]))/gi

export function avatar(user) {
    return user.avatarUrl === null
        ? `<figure class="avatar" data-initial="${user.username[0]}"></figure>`
        : `<img class="avatar" src="${user.avatarUrl}" alt="${user.username}'s avatar">`
}

/**
 * @param {Date|string} date
 */
export function ago(date) {
    const now = new Date()
    if (!(date instanceof Date)) {
        date = new Date(date)
    }
    let diff = (now.getTime() - date.getTime()) / 1000
    if (diff <= 60) {
        return 'Just now'
    }
    if ((diff /= 60) < 60) {
        return (diff | 0) + 'm'
    }
    if ((diff /= 60) < 24) {
        return (diff | 0) + 'h'
    }
    if ((diff /= 24) < 7) {
        return (diff | 0) + 'd'
    }
    const text = String(date).split(' ')[1] + ' ' + date.getDate()
    if (diff > 182 && now.getFullYear() !== date.getFullYear()) {
        return `${text}, ${date.getFullYear()}`
    }
    return text
}

/**
 * @param {string} str
 */
export function escapeHTML(str) {
    return str
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;')
        .replace(/'/g, '&#039;')
}

/**
 * @param {string} str
 */
export function linkify(str) {
    return str.replace(urlRegExp, (_, u) => `<a href="${/^[a-zA-Z]{1,6}:/.test(u) ? u : 'http://' + u}" target="_blank" rel="noopener">${decodeURI(u)}</a>`)
}

/**
 * @param {string} src
 */
export function loadScript(src) {
    const script = document.createElement('script')
    script.src = src
    script.async = true

    return new Promise((resolve, reject) => {
        script.onload = () => {
            script.remove()
            resolve()
        }
        script.onerror = err => {
            script.remove()
            reject(err)
        }
        document.head.appendChild(script)
    })
}

export function loadEventSourcePolyfill() {
    return loadScript('https://unpkg.com/event-source-polyfill@0.0.12/src/eventsource.min.js')
}
