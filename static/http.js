import { isAuthenticated } from './auth.js';

/**
 * @param {Response} res
 */
async function handleResponse(res) {
    const body = await res.clone().json().catch(() => res.text())

    if (res.status === 401) {
        localStorage.removeItem('auth_user')
        localStorage.removeItem('token')
        localStorage.removeItem('expires_at')
    }

    if (!res.ok) {
        const message = typeof body === 'object' && body !== null && 'message' in body
            ? body.message
            : typeof body === 'string' && body !== ''
                ? body
                : res.statusText
        throw Object.assign(new Error(message), {
            url: res.url,
            statusCode: res.status,
            statusText: res.statusText,
            headers: res.headers,
            body,
        })
    }

    return body
}

function getAuthHeader() {
    return isAuthenticated()
        ? { authorization: `Bearer ${localStorage.getItem('token')}` }
        : {}
}

export default {
    /**
     * @param {string} url
     * @param {{[x: string]: string}=} headers
     */
    get(url, headers) {
        return fetch(url, {
            headers: Object.assign(getAuthHeader(), headers),
        }).then(handleResponse)
    },

    /**
     * @param {string} url
     * @param {(FormData|File|{[x: string]: string})=} body
     * @param {{[x: string]: string}=} headers
     */
    post(url, body, headers) {
        const init = {
            method: 'POST',
            headers: getAuthHeader(),
        }
        if (typeof body === 'object' && body !== null) {
            init.body = JSON.stringify(body)
            init.headers['content-type'] = 'application/json; charset=utf-8'
        }
        Object.assign(init.headers, headers)
        return fetch(url, init).then(handleResponse)
    },

    /**
     * @param {string} url
     * @param {function} callback
     */
    subscribe(url, callback) {
        const urlWithToken = new URL(url, location.origin)
        if (isAuthenticated()) {
            urlWithToken.searchParams.set('token', localStorage.getItem('token'))
        }
        const eventSource = new EventSource(urlWithToken.toString())
        eventSource.onmessage = ev => {
            let data
            try {
                data = JSON.parse(ev.data)
            } catch (err) {
                console.error('could not parse message data as JSON:', err)
                return
            }
            callback(data)
        }
        const unsubscribe = () => {
            eventSource.close()
        }
        return unsubscribe
    },
}
