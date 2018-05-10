#!/usr/bin/env python
# -*- encoding: utf-8 -*-
from __future__ import print_function
import os
import sys
import json
import time
import requests
import eventlet
import argparse
import traceback
import inspect

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
    def __init__(self, url, method, count):
        self._executor = futurist.GreenThreadPoolExecutor(max_workers=1000)
        self.url = url
        self.method = method
        self.count = count

    def run(self):
        if self.method == 'all':
            for func in inspect.getmembers(self):
                if func[0].startswith('test_'):
                    getattr(self, func[0])()
        elif hasattr(self, 'test_%s' % self.method):
            getattr(self, 'test_%s' % self.method)()
        else:
            print('Could not find method test_%s' % self.method)

    def list(self):
        print("Test function list:\n")
        for func in inspect.getmembers(self):
            if func[0].startswith('test_'):
                print(func[0])

    @elaspe_run(desc="post")
    def test_post(self):
        def _test_post(data):
            requests.post("%s/nodes" % self.url, data=json.dumps(data))

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
        requests.post("%s/bulk/nodes" % self.url, data=json.dumps(data))

    @elaspe_run(desc="delete")
    def test_delete(self):
        def _test_delete(i):
            requests.delete('%s/nodes/fakenode%d' % (self.url, i))

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
        requests.delete('%s/bulk/nodes' % self.url, data=json.dumps(data))

    @elaspe_run(desc="list")
    def test_list(self):
        requests.get('%s/nodes' % self.url)

    @elaspe_run(desc="put_on")
    def test_put_on(self):
        def _test_put(i):
            requests.delete('%s/nodes/fakenode%d?state=on' % (self.url, i))

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
        requests.delete('%s/bulk/nodes?state=on' % self.url,
                        data=json.dumps(data))

    @elaspe_run(desc="put_off")
    def test_put_off(self):
        def _test_put(i):
            requests.delete('%s/nodes/fakenode%d?state=off' % (self.url, i))

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
        requests.delete('%s/bulk/nodes?state=off' % self.url,
                        data=json.dumps(data))

    @elaspe_run(desc="show")
    def test_show(self):
        def _test_get(i):
            requests.get('%s/nodes/fakenode%d' % (self.url, i))

        futures = []
        for i in range(self.count):
            future = self._executor.submit(_test_get, i)
            futures.append(future)
        waiters.wait_for_all(futures, 3600)


class BenchmarkAPI(object):
    def get_base_parser(self):
        parser = argparse.ArgumentParser(
            prog='benchapi',
            epilog='See "benchapi help COMMAND" '
                   'for help on a specific command.',
            add_help=False,
            formatter_class=HelpFormatter,
        )
        parser.add_argument('-h', '--help',
                            action='store_true',
                            help=argparse.SUPPRESS)
        parser.add_argument('-m', '--method',
                            help="The rest api method, could be post, delete, "
                                 "bulk_post, bulk_delete, all",
                            default='all',
                            type=str)
        parser.add_argument('-c', '--count',
                            help="The number of nodes in the test case",
                            type=int,
                            default=1)
        parser.add_argument('--url',
                            help="http url of goconserver master",
                            type=str,
                            default='http://localhost:12429')
        parser.add_argument('-l', '--list',
                            help="List the test functions for benchmark api",
                            action='store_true')
        return parser

    def do_help(self, args):
        self.parser.print_help()

    def main(self, argv):
        self.parser = self.get_base_parser()
        (options, args) = self.parser.parse_known_args(argv)

        if options.help:
            self.do_help(options)
            return 0

        api = APItest(options.url, options.method, options.count)
        if options.list:
            api.list()
            return 0

        api.run()


class HelpFormatter(argparse.HelpFormatter):
    def start_section(self, heading):
        # Title-case the headings
        heading = '%s%s' % (heading[0].upper(), heading[1:])
        super(HelpFormatter, self).start_section(heading)


if __name__ == "__main__":
    try:
        BenchmarkAPI().main(sys.argv[1:])
    except KeyboardInterrupt:
        print("... terminating benchmark testing", file=sys.stderr)
        sys.exit(130)
    except Exception as e:
        print(traceback.format_exc())
        sys.exit(1)
