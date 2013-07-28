$(function() {
    var prevCh = null, prevTime = 0, modal = false;

    var exportPat = /^[^_][^-]*$/

    function exports() {
        var result = []
        $('*[id]').each(function() {
            var id = $(this).attr('id');
            if (exportPat.test(id)) {
                result.push(id);
            }
        });
        return result;
    }

    $('#_jump').on({
        hide: function() { $('#_jump_text').blur(); },
        shown: function() { $('#_jump_text').val('').focus(); },
    });

    $('#_jump_form').on({
        submit: function(e) {
            $('#_jump').modal('hide');
            window.location.href = '#' + $('#_jump_text').val();
            return false;
        }
    });

    $('.modal').on({
        show: function() { modal = true; },
        hidden: function() { modal = false; }
    });

    $(document).on('keypress', function(e) {
        var combo = e.timeStamp - prevTime <= 1000;
        prevTime = 0;

        if (modal) {
            return true;
        }

        var t = e.target.tagName
        if (t == 'INPUT' || 
            t == 'SELECT' || 
            t == 'TEXTAREA' ) {
            return true;
        }

        if (e.target.contentEditable && e.target.contentEditable == 'true') {
            return true;
        }

        var ch = String.fromCharCode(e.which);

        if (combo) {
            switch (prevCh + ch) {
            case "gg":
                $('html,body').animate({scrollTop: 0},'fast');
                return false;
            case "gb":
                $('html,body').animate({scrollTop: $(document).height()},'fast');
                return false;
            case "gi":
                if ($('#_index').length > 0) {
                    $('html,body').animate({scrollTop: $("#_index").offset().top},'fast');
                    return false;
                }
            case "ge":
                if ($('#_examples').length > 0) {
                    $('html,body').animate({scrollTop: $("#_examples").offset().top},'fast');
                    return false;
                }
            }
        }

        switch (ch) {
        case "/":
            $('#_search').focus();
            return false;
        case "?":
            $('#_shortcuts').modal();
            return false;
        case  ".":
            if ($('#_jump').length > 0) {
                window.setTimeout(function() { 
                    $('#_jump_text').typeahead({source: exports()});
                    $('#_jump').modal(); 
                }, 0);
                return false;
            }
        }

        prevCh = ch
        prevTime = e.timeStamp
        return true;
    });

    var thData;

    function searchSource(query, process) {
        if (thData) {
            return thData.items;
        }
        return $.get('/-/typeahead', { q: query }, function (data) {
            thData = data
            return process(data.items);
        });
    }

    function searchSorter(items) {
        var p1 = [], p2 = [], p3 = [];
        var q = this.query.toLowerCase();
        var item;
        while (item = items.shift()) {
            var i = item.toLowerCase();
            var n = i.substring(i.lastIndexOf("/")).indexOf(q);
            if (n == 0) {
                p1.push(item);
            } else if (n > 0) {
                p2.push(item);
            } else if (i.indexOf(q) >= 0) {
                p3.push(item);
            }
        }
        return p1.concat(p2, p3);
    }

    //$('#_searchBox').typeahead({source: searchSource, sorter: searchSorter});
    //
    $('span.timeago').timeago();
    if (window.location.hash.substring(0, 10) == '#_example_') {
       $('#_ex_' + window.location.hash.substring(10)).addClass('in').height('auto');
    }

    var highlighted;

    function highlightHash() {
        if (highlighted) {
            highlighted.removeClass("highlight");
        }
        if (window.location.hash) {
            highlighted = $('#_file ' + window.location.hash.replace(/([.])/g,'\\$1'));
            highlighted.addClass("highlight");
        } else {
            highlighted = null;
        }
    }

    window.onhashchange = highlightHash;
    highlightHash();

});
