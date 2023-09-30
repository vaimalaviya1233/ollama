import os
import json
import requests
from urllib.parse import urljoin

base = os.getenv('OLLAMA_HOST', 'http://localhost:11434')


def do(method, url, **kwargs):
    response = requests.request(method, urljoin(base, url), **kwargs)
    response.raise_for_status()
    return response


def stream_response(response):
    for lines in response.iter_lines():
        for line in lines.splitlines():
            chunk = json.loads(line)
            if error := chunk.get('error'):
                raise Exception(error)

            yield chunk


def generate(model_name, prompt, system='', template='', context=[], options={}, stream=False):
    '''
    Generate a response for a given prompt with a provided model. This is a streaming endpoint, so
    will be a series of responses. The final response object will include statistics and additional
    data from the request.
    '''
    response = do('POST', '/api/generate', stream=True, json={
        k: v for k, v in {
            "model": model_name,
            "prompt": prompt,
            "system": system,
            "template": template,
            "context": context,
            "options": options
        }.items() if v
    })

    if stream:
        return stream_response(response)

    text = ''
    for chunk in stream_response(response):
        if r := chunk.get('response', ''):
            text += r
        if chunk.get('done'):
            chunk['response'] = text
            chunk['status'] = 'success'
            return chunk


def create(model_name, model_path, stream=False):
    '''
    Create a model from a Modelfile.
    '''
    response = do('POST', '/api/create', json={'name': model_name, 'path': model_path})

    if stream:
        return stream_response(response)

    for chunk in stream_response(response):
        pass

    return chunk


def pull(model_name, insecure=False, stream=False):
    '''
    Pull a model from a the model registry. Cancelled pulls are resumed from where they left off,
    and multiple calls to will share the same download progress.
    '''
    response = do('POST', '/api/pull', stream=True, json={'name': model_name, 'insecure': insecure})

    if stream:
        return stream_response(response)

    layers = {}
    for chunk in stream_response(response):
        if digest := chunk.pop('digest', None):
            layers[digest] = chunk

    layers['status'] = 'success'
    return layers


def push(model_name, insecure=False, stream=False):
    '''
    Push a model to the model registry.
    '''
    response = do('POST', '/api/push', stream=True, json={'name': model_name, 'insecure': insecure})

    if stream:
        return stream_response(response)

    layers = {}
    for chunk in stream_response(response):
        if digest := chunk.pop('digest', None):
            layers[digest] = chunk

    layers['status'] = 'success'
    return layers


def list():
    '''
    List models that are available locally.
    '''
    response = do('GET', '/api/tags')
    return response.json().get('models') or []


def copy(source, destination):
    '''
    Copy a model. Creates a model with another name from an existing model.
    '''
    response = do('POST', '/api/copy', json={'source': source, 'destination': destination})
    return {'status': 'success' if response.status_code == 200 else 'error'}


def delete(model_name):
    '''
    Delete a model and its data.
    '''
    response = do('DELETE', '/api/delete', json={'name': model_name})
    return {'status': 'success' if response.status_code == 200 else 'error'}


def show(model_name):
    '''
    Show info about a model.
    '''
    return do('POST', '/api/show', json={'name': model_name}).json()


def ping():
    '''
    Ping the server to check if it is running.
    '''
    response = do('HEAD', '/')
    return {'status': 'success' if response.status_code == 200 else 'error'}
