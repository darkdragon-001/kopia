#!/usr/bin/python3

# use debian package release
def deb_package(version):
    import urllib.request, hashlib
    url = 'https://github.com/kopia/kopia/releases/download/{}/kopia-ui_{}_amd64.deb'.format(version, version.strip('v'))
    try:
        response = urllib.request.urlopen(url)
    except:
        print('Error downloading release {} from {}.'.format(version, url))
        raise
    else:
        bytes = response.read()
        sha256 = hashlib.sha256(bytes).hexdigest()
    return url, sha256

# manifest
def manifest(url, sha256):
    import json
    with open('io.kopia.KopiaUI.json', 'r+') as f:
        manifest = json.load(f)
        manifest['modules'][0]['sources'][0]['url'] = url
        manifest['modules'][0]['sources'][0]['sha256'] = sha256
        # overwrite
        f.seek(0)
        json.dump(manifest, f, indent=4)
        f.truncate()

# appdata
def appdata(version, date):
    import xml.etree.ElementTree
    appdata_filename = 'io.kopia.KopiaUI.appdata.xml'
    appdata = xml.etree.ElementTree.parse(appdata_filename)
    releases = appdata.getroot().find('releases')
    releases.insert(0, xml.etree.ElementTree.Element('release', {'version': version, 'date': date}))
    #appdata.write(appdata_filename, encoding='UTF-8', xml_declaration=True)  # does not pretty print
    def prettyXml(element):
        import xml.dom.minidom
        xml_str = xml.etree.ElementTree.tostring(appdata.getroot(), encoding='UTF-8', xml_declaration=True)
        minidom_xml = xml.dom.minidom.parseString(xml_str)
        return '\n'.join([line for line in minidom_xml.toprettyxml(indent=' '*2, encoding='UTF-8').decode('utf-8').split('\n') if line.strip()])
    with open(appdata_filename, 'w') as f:
        f.write(prettyXml(appdata.getroot()))

# main entry point
if __name__ == "__main__":
    # version parameter
    import re, sys
    if len(sys.argv) != 2:
        print('This script must be called with exactly one argument describing the version.')
        sys.exit(1)
    version = sys.argv[1]
    m = re.match(r'^v?([0-9]+\.[0-9]+\.[0-9]+)$', version)
    if m is None:
        print('The version argument must be of format X.Y.Z or vX.Y.Z')
        sys.exit(1)
    # date
    import datetime
    date = datetime.datetime.today().strftime('%Y-%m-%d')  # TODO get from Github

    # manifest
    manifest(*deb_package(version))
    # appdata
    appdata(version, date)
