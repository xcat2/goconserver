# -*- encoding: utf-8 -*-
# !/usr/bin/python
from __future__ import print_function
import os
import sys
import json
import time
import requests
import eventlet

eventlet.monkey_patch(os=False)
import futurist
from futurist import waiters


def elaspe_run(desc=''):
    def _wrap(func):
        def wrap(*args, **kwargs):
            start_time = time.time()
            func(*args, **kwargs)
            end_time = time.time()
            print("*** %s Elapsed time: %f ***" % (desc,
                                                   end_time - start_time))

        return wrap

    return _wrap


class APItest(object):
    def __init__(self, count):
        self._executor = futurist.GreenThreadPoolExecutor(max_workers=1000)
        self.count = count
        if 'consoleserver_host' in os.environ:
            self.host = os.environ["consoleserver_host"]
        else:
            self.host = 'http://localhost:8089'

    @elaspe_run(desc="post")
    def test_post(self):
        def _test_post(data):
            requests.post("%s/nodes" % self.host, data=json.dumps(data))

        futures = []
        for i in range(self.count):
            data = {'name': 'fakenode%d' % i, 'driver': 'ssh',
                    'ondemand': True,
                    'params': {'user': 'long', 'host': '11.114.120.101',
                               'port': '22',
                               'private_key': '/Users/longcheng/.ssh/id_rsa'}}
            future = self._executor.submit(_test_post, data)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)

    @elaspe_run(desc="bulk_post")
    def test_bulk_post(self):
        data = {'nodes': []}
        for i in range(self.count):
            item = {'name': 'fakenode%d' % i, 'driver': 'ssh',
                    'ondemand': True,
                    'params': {'user': 'long', 'host': '11.114.120.101',
                               'port': '22',
                               'private_key': '/Users/longcheng/.ssh/id_rsa'}}
            data['nodes'].append(item)
        requests.post("%s/bulk/nodes" % self.host, data=json.dumps(data))

    @elaspe_run(desc="delete")
    def test_delete(self):
        def _test_delete(i):
            requests.delete('%s/nodes/fakenode%d' % (self.host, i))

        futures = []
        for i in range(self.count):
            future = self._executor.submit(_test_delete, i)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)

    @elaspe_run(desc="bulk_delete")
    def test_bulk_delete(self):
        data = {'nodes': []}
        for i in range(self.count):
            item = {'name': 'fakenode%d' % i}
            data['nodes'].append(item)
        requests.delete('%s/bulk/nodes' % self.host, data=json.dumps(data))

    @elaspe_run(desc="list")
    def test_list(self):
        requests.get('%s/nodes' % self.host)

    @elaspe_run(desc="put_on")
    def test_put_on(self):
        def _test_put(i):
            requests.delete('%s/nodes/fakenode%d?state=on' % (self.host, i))

        futures = []
        for i in range(self.count):
            future = self._executor.submit(_test_put, i)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)

    @elaspe_run(desc="bulk_put_on")
    def test_bulk_put_on(self):
        data = {'nodes': []}
        for i in range(self.count):
            item = {'name': 'fakenode%d' % i}
            data['nodes'].append(item)
        requests.delete('%s/bulk/nodes?state=on' % self.host,
                        data=json.dumps(data))

    @elaspe_run(desc="put_off")
    def test_put_off(self):
        def _test_put(i):
            requests.delete('%s/nodes/fakenode%d?state=off' % (self.host, i))

        futures = []
        for i in range(self.count):
            future = self._executor.submit(_test_put, i)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)

    @elaspe_run(desc="bulk_put_off")
    def test_bulk_put_off(self):
        data = {'nodes': []}
        for i in range(self.count):
            item = {'name': 'fakenode%d' % i}
            data['nodes'].append(item)
        requests.delete('%s/bulk/nodes?state=off' % self.host,
                        data=json.dumps(data))

    @elaspe_run(desc="show")
    def test_show(self):
        def _test_get(i):
            requests.get('%s/nodes/fakenode%d' % (self.host, i))

        futures = []
        for i in range(self.count):
            future = self._executor.submit(_test_get, i)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)


if __name__ == "__main__":
    if len(sys.argv) == 3:
        method = str(sys.argv[1])
        count = int(sys.argv[2])
        api = APItest(count)

        if method == 'post':
            api.test_post()
        elif method == 'delete':
            api.test_delete()
        elif method == 'bulk_post':
            api.test_bulk_post()
        elif method == 'bulk_delete':
            api.test_bulk_delete()
    else:
        count = 1000
        api = APItest(count)
        api.test_bulk_post()
        api.test_list()
        api.test_show()
        api.test_put_on()
        api.test_put_off()
        api.test_bulk_put_on()
        api.test_bulk_put_off()
        api.test_bulk_delete()
        api.test_post()
        api.test_delete()
